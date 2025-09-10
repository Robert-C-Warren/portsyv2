package main

import (
	backend "Portsy/backend"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env: %s", key)
	}
	return v
}

func checkFirestore(ctx context.Context, meta *backend.MetaStore) error {
	testProj := "portsy-selftest"
	commit := backend.CommitMeta{
		ID:        uuid.NewString(),
		Message:   "selftest",
		Timestamp: time.Now().Unix(),
	}
	state := backend.ProjectState{
		ProjectName: testProj,
		ProjectPath: "/dev/null",
		Files:       []backend.FileEntry{},
		CreatedAt:   time.Now().Unix(),
	}
	if err := meta.UpsertLatestState(ctx, testProj, state, commit); err != nil {
		return fmt.Errorf("firestore write failed: %w", err)
	}
	_, cm, err := meta.GetLatestState(ctx, testProj)
	if err != nil {
		return fmt.Errorf("firestore read failed: %w", err)
	}
	if cm == nil || cm.ID != commit.ID {
		return fmt.Errorf("firestore roundtrip mismatch")
	}
	log.Println("✓ Firestore: write/read ok")
	return nil
}

func checkR2(ctx context.Context, r2 *backend.R2Client) error {
	key := fmt.Sprintf("selftest/%s.txt", uuid.NewString())
	data := []byte("portsy r2 ping")
	if err := r2.UploadReader(ctx, bytes.NewReader(data), key); err != nil {
		return fmt.Errorf("r2 upload failed: %w", err)
	}
	ok, err := r2.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("r2 head failed: %w", err)
	}
	if !ok {
		return fmt.Errorf("r2 object not found after upload")
	}
	tmp := filepath.Join(os.TempDir(), "portsy_r2_download.txt")
	if err := r2.DownloadTo(ctx, key, tmp); err != nil {
		return fmt.Errorf("r2 download failed: %w", err)
	}
	_ = os.Remove(tmp)
	if err := r2.Delete(ctx, key); err != nil {
		return fmt.Errorf("r2 delete failed: %w", err)
	}
	log.Println("✓ R2: upload/head/download/delete ok")
	return nil
}

// smokePush uploads all files using the SAME key builder as production,
// then BeginCommit -> FinalizeCommit with verify(hash->key).
func smokePush(ctx context.Context, meta *backend.MetaStore, r2 *backend.R2Client, projectName, projectPath, message string) {
	// 1) Build manifest/state
	st, err := backend.BuildManifest(projectPath)
	if err != nil {
		log.Fatalf("manifest: %v", err)
	}
	log.Printf("manifest: %d file(s)", len(st.Files))

	// 2) Idempotent upload/ensure every blob
	up := 0
	for i := range st.Files {
		fe := &st.Files[i]
		fe.R2Key = r2.BuildKey(projectName, fe.Hash)
		abs := filepath.Join(projectPath, filepath.FromSlash(fe.Path))

		if err := r2.UploadIfMissing(ctx, abs, fe.R2Key); err != nil {
			log.Fatalf("upload %s: %v", fe.R2Key, err)
		}
		up++
	}
	log.Printf("attempted uploads=%d (idempotent)", up)

	// 3) Begin commit (pending)
	cm := backend.CommitMeta{
		ID:        uuid.NewString(),
		Message:   message,
		Timestamp: time.Now().Unix(),
		Status:    "pending",
	}
	if err := meta.BeginCommit(ctx, projectName, cm, st); err != nil {
		log.Fatalf("begin commit: %v", err)
	}
	log.Printf("commit %s: pending", cm.ID)

	// 4) Finalize with verify(hash -> SAME key)
	verify := func(ctx context.Context, sha string) error {
		key := r2.BuildKey(projectName, sha)
		ok, err := r2.Exists(ctx, key)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("missing blob %s", key)
		}
		return nil
	}
	if err := meta.FinalizeCommit(ctx, projectName, cm, st, verify); err != nil {
		log.Fatalf("finalize: %v", err)
	}
	log.Printf("commit %s: FINAL ✓", cm.ID)
}

func main() {
	// Load .env with override semantics
	_ = godotenv.Overload(".env", "../.env", "../../.env")

	// Normalize GOOGLE_APPLICATION_CREDENTIALS to absolute path if relative
	cred := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if strings.HasPrefix(cred, ".") {
		if abs, err := filepath.Abs(cred); err == nil {
			cred = abs
		}
	}
	if _, err := os.Stat(cred); err != nil {
		log.Fatalf("GOOGLE_APPLICATION_CREDENTIALS not found at %q: %v", cred, err)
	}

	metaCfg := backend.MetaStoreConfig{
		GCPProjectID:      mustEnv("GCP_PROJECT_ID"),
		ServiceAccountKey: cred,
	}

	var (
		mode        = flag.String("mode", "check", "check | scan | push | pull | rollback | watch | pending | diff | smoke")
		root        = flag.String("root", "", "projects root (scan/push/watch)")
		projectName = flag.String("project", "", "project name (push/pull/rollback/watch/smoke)")
		msg         = flag.String("msg", "test push", "commit message (push/smoke)")
		dest        = flag.String("dest", "", "destination for pull/rollback (defaults to <root>/<project>)")
		commitID    = flag.String("commit", "", "commit ID (rollback or pull specific commit)")
		force       = flag.Bool("force", false, "allow deleting local files not in target state (pull)")
		jsonOut     = flag.Bool("json", false, "emit JSON (for scan|pending|diff)")
		autoPush    = flag.Bool("autopush", false, "if set, push automatically after collect (watch)")
	)
	flag.Parse()

	ctx := context.Background()

	meta, err := backend.NewMetaStore(ctx, metaCfg)
	if err != nil {
		log.Fatalf("firestore init: %v", err)
	}
	defer meta.Close()

	r2Cfg := backend.R2Config{
		AccountID: mustEnv("R2_ACCOUNT_ID"),
		AccessKey: mustEnv("R2_ACCESS_KEY"),
		SecretKey: mustEnv("R2_SECRET_KEY"),
		Bucket:    mustEnv("R2_BUCKET"),
		Region:    os.Getenv("R2_REGION"),
	}
	r2, err := backend.NewR2(ctx, r2Cfg)
	if err != nil {
		log.Fatalf("r2 init: %v", err)
	}

	log.Printf("cfg: proj=%s r2[acct=%s bucket=%s region=%s key=%s...]",
		metaCfg.GCPProjectID, r2Cfg.AccountID, r2Cfg.Bucket,
		func() string {
			if r2Cfg.Region == "" {
				return "auto"
			}
			return r2Cfg.Region
		}(),
		func(s string) string {
			if len(s) < 6 {
				return "***"
			}
			return s[:3] + "…" + s[len(s)-3:]
		}(r2Cfg.AccessKey),
	)

	switch *mode {
	case "check":
		if err := checkFirestore(ctx, meta); err != nil {
			log.Fatal(err)
		}
		if err := checkR2(ctx, r2); err != nil {
			log.Fatal(err)
		}
		log.Println("All checks passed 🎉")

	case "smoke":
		if *root == "" || *projectName == "" {
			log.Fatal("smoke requires -root and -project")
		}
		projectPath := filepath.Join(*root, *projectName)
		smokePush(ctx, meta, r2, *projectName, projectPath, *msg)
		return

	case "scan":
		if *root == "" {
			fmt.Println(`usage: -mode=scan -root "<path>" [-json]`)
			return
		}
		projs, err := backend.ScanProjects(*root)
		if err != nil {
			fmt.Printf("scan error: %v\n", err)
			return
		}
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(projs)
			return
		}
		for _, p := range projs {
			fmt.Printf("- %s (HasPortsy=%v)\n", p.Name, p.HasPortsy)
		}

	case "push":
		if *root == "" || *projectName == "" {
			log.Fatal("push requires -root and -project")
		}
		projectPath := filepath.Join(*root, *projectName)

		projs, err := backend.ScanProjects(*root)
		if err != nil {
			log.Fatal(err)
		}
		var sel *backend.AbletonProject
		for i := range projs {
			if projs[i].Name == *projectName {
				sel = &projs[i]
				break
			}
		}
		if sel == nil {
			log.Fatalf("project %q not found under %s", *projectName, *root)
		}

		cm := backend.CommitMeta{
			ID:        uuid.NewString(),
			Message:   *msg,
			Timestamp: time.Now().Unix(),
		}
		if err := backend.PushProject(ctx, meta, r2, *sel, cm); err != nil {
			log.Fatal(err)
		}
		if ps, err := backend.BuildManifest(projectPath); err == nil {
			algo := ps.Algo
			if algo == "" {
				algo = "sha256"
			}
			_ = backend.WriteCacheFromState(projectPath, ps, algo)
		}
		log.Println("Push completed ✓")

	case "pull":
		if *projectName == "" {
			log.Fatal("pull requires -project")
		}
		dst := *dest
		if dst == "" {
			base := *root
			if base == "" {
				cwd, _ := os.Getwd()
				base = cwd
			}
			dst = filepath.Join(base, *projectName)
		}
		if _, err := backend.PullProject(ctx, meta, r2, *projectName, dst, *commitID, *force); err != nil {
			log.Fatal(err)
		}
		if ps, err := backend.BuildManifest(dst); err == nil {
			algo := ps.Algo
			if algo == "" {
				algo = "sha256"
			}
			_ = backend.WriteCacheFromState(dst, ps, algo)
		}
		log.Printf("Pulled %q into %s ✓", *projectName, dst)

	case "rollback":
		if *projectName == "" || *commitID == "" {
			log.Fatal("rollback requires -project and -commit")
		}
		dst := *dest
		if dst == "" {
			base := *root
			if base == "" {
				cwd, _ := os.Getwd()
				base = cwd
			}
			dst = filepath.Join(base, *projectName)
		}
		if err := backend.RollbackProject(ctx, meta, r2, *projectName, dst, *commitID); err != nil {
			log.Fatal(err)
		}
		log.Printf("Rolled back %q to commit %s into %s ✓", *projectName, *commitID, dst)

	case "watch":
		rootFlag := flag.Lookup("root")
		projectFlag := flag.Lookup("project")
		if rootFlag == nil || rootFlag.Value.String() == "" {
			fmt.Println(`usage: -mode=watch -root "<path>" [-project "<name>"] [-autopush]`)
			return
		}
		rootPath := rootFlag.Value.String()

		onSave := func(evt backend.SaveEvent) {
			fmt.Printf("[watch] %s: %s saved @ %s\n", evt.ProjectName, filepath.Base(evt.ALSPath), evt.DetectedAt.Format(time.RFC3339))
			copied, err := backend.CollectNewSamples(context.Background(), evt.ProjectPath, evt.ALSPath)
			if err != nil {
				fmt.Printf("[collect] error: %v\n", err)
			} else if len(copied) > 0 {
				fmt.Printf("[collect] copied %d sample(s) into Samples/Imported\n", len(copied))
			} else {
				fmt.Printf("[collect] no new samples to copy\n")
			}
			doPush := *autoPush
			if !doPush {
				fmt.Printf("Push changes to remote for \"%s\"? [y/N]: ", evt.ProjectName)
				var resp string
				_, _ = fmt.Scanln(&resp)
				resp = strings.TrimSpace(strings.ToLower(resp))
				doPush = resp == "y" || resp == "yes"
			}
			if !doPush {
				return
			}
			exe, err := os.Executable()
			if err != nil {
				fmt.Printf("[push] cannot resolve executable: %v\n", err)
				return
			}
			msg := fmt.Sprintf("autosync: %s", time.Now().Format(time.RFC3339))
			cmd := exec.Command(exe, "-mode=push", "-root", rootPath, "-project", evt.ProjectName, "-msg", msg)
			cmd.Env = os.Environ() // inherit creds/env
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("[push] error: %v\n", err)
				return
			}
			fmt.Printf("[push] %s success.\n", evt.ProjectName)
		}

		// base watch context on outer ctx so future cancel hooks work
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		proj := ""
		if projectFlag != nil {
			proj = strings.TrimSpace(projectFlag.Value.String())
		}
		if proj == "" {
			fmt.Printf("Watching ALL projects under %s … (Ctrl+C to stop)\n", rootPath)
			if err := backend.WatchAllProjects(ctx, rootPath, 750*time.Millisecond, onSave); err != nil {
				fmt.Printf("watch error: %v\n", err)
			}
			return
		}
		projectPath := filepath.Join(rootPath, proj)
		fmt.Printf("Watching %s … (Ctrl+C to stop)\n", projectPath)
		if err := backend.WatchProjectALS(ctx, proj, projectPath, 750*time.Millisecond, onSave); err != nil {
			fmt.Printf("watch error: %v\n", err)
		}

	case "pending":
		if *root == "" {
			fmt.Println(`usage: -mode=pending -root "<path>" [-json]`)
			return
		}
		changes, err := backend.ChangedProjectsSinceCache(*root)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			return
		}
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(changes)
			return
		}
		if len(changes) == 0 {
			fmt.Println("No local changes since last cache.")
			return
		}
		for _, c := range changes {
			fmt.Printf("- %s  (+%d ~%d -%d)  total %d\n", c.Name, c.Added, c.Modified, c.Deleted, c.Total)
		}

	case "diff":
		if *root == "" || *projectName == "" {
			fmt.Println(`usage: -mode=diff -root "<path>" -project "<name>" [-json]`)
			return
		}
		projectPath := filepath.Join(*root, *projectName)
		ps, err := backend.BuildManifest(projectPath)
		if err != nil {
			fmt.Printf("manifest error: %v\n", err)
			return
		}
		cur := backend.ManifestFromState(ps)
		lc, _ := backend.LoadLocalCache(projectPath)
		changes := backend.DiffManifests(cur, lc.Manifest)
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(changes)
			return
		}
		if len(changes) == 0 {
			fmt.Println("No local changes since last cache.")
			return
		}
		for _, ch := range changes {
			fmt.Printf("%-8s %s\n", ch.Type, ch.Path)
		}

	default:
		log.Fatalf("unknown mode: %s", *mode)
	}
}
