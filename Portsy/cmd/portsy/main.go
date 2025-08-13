package main

import (
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

	"Portsy/backend" // <-- matches the module name you'll init below

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
	testProj := "portsy-selftest" // <- avoid leading "__"
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

func main() {
	// Force .env values to override any pre-set env vars:
	_ = godotenv.Overload(".env", "../.env", "../../.env")

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

	// add flags
	var (
		mode        = flag.String("mode", "check", "check | scan | push | pull | rollback | watch | pending | diff")
		root        = flag.String("root", "", "projects root (scan/push/watch)")
		projectName = flag.String("project", "", "project name (push/pull/rollback/watch)")
		msg         = flag.String("msg", "test push", "commit message (push)")
		dest        = flag.String("dest", "", "destination directory for pull/rollback (defaults to <root>/<project>)")
		commitID    = flag.String("commit", "", "commit ID (for rollback or pull specific commit)")
		force       = flag.Bool("force", false, "allow deleting local files not in target state (pull)")
		jsonOut     = flag.Bool("json", false, "emit JSON (for scan|pending|diff)")
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

	switch *mode {
	case "check":
		if err := checkFirestore(ctx, meta); err != nil {
			log.Fatal(err)
		}
		if err := checkR2(ctx, r2); err != nil {
			log.Fatal(err)
		}
		log.Println("All checks passed 🎉")
		// LIST PROJECTS (LOCAL)
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

		// define projectPath so we can build the manifest/cache later
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

		// update local cache AFTER a successful push
		if ps, err := backend.BuildManifest(projectPath); err == nil {
			_ = backend.WriteCacheFromState(projectPath, ps)
		}

		log.Println("Push completed ✓")

	case "pull":
		if *projectName == "" {
			log.Fatal("pull requires -project")
		}
		// default dest: <root>/<project>, falls back to current dir/<project> if root is unset
		dst := *dest
		if dst == "" {
			base := *root
			if base == "" {
				cwd, _ := os.Getwd()
				base = cwd
			}
			dst = filepath.Join(base, *projectName)
		}
		if err := backend.PullProject(ctx, meta, r2, *projectName, dst, *commitID, *force); err != nil {
			log.Fatal(err)
		}
		if ps, err := backend.BuildManifest(dst); err == nil {
			_ = backend.WriteCacheFromState(dst, ps)
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
		autoPush := flag.Bool("autopush", false, "if set, push automatically after collect")
		flag.Parse()

		if rootFlag == nil || rootFlag.Value.String() == "" {
			fmt.Println(`usage: -mode=watch -root "<path>" [-project "<name>"] [-autopush]`)
			return
		}

		rootPath := rootFlag.Value.String()

		// define the onSave handler once (works for single or multi)
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

			// Ask to push (or auto-push)
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
			cmd := exec.Command(exe,
				"-mode=push",
				"-root", rootPath,
				"-project", evt.ProjectName,
				"-msg", msg,
			)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("[push] error: %v\n", err)
				return
			}
			fmt.Printf("[push] %s success.\n", evt.ProjectName)
		}
		ctx, cancel := context.WithCancel(context.Background())
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
		// WHICH PROJECTS CHANGED SINCE LAST CACHE
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

		// FILE-LEVEL DIFF FOR ONE PROJECT VS LOCAL CACHE
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
