import * as Main from '../../wailsjs/go/main/App'

// ---- PUSH ----
export async function listPushableProjects() {
    if (Main.listPushableProjects) return Main.listPushableProjects();
    if (Main.ListProjectsWithLocalChanges) return Main.ListProjectsWithLocalChanges();
    throw new Error('No ListPushableProjects/ListProjectsWithLocalChanges binding found in go/main/App');
}
export async function getDiffForProject(projectName) {
    if (Main.getDiffForProject) return Main.getDiffForProject();
    if (Main.GetDiff) return Main.GetDiff();
    throw new Error('No GetDiffForProject/GetDiff binding found in go/main/App');
}
export async function pushProject(projectName, message) {
    if (Main.PushProjectWithMessage) return Main.PushProjectWithMessage(projectName, message);
    if (Main.PushProject) return Main.PushProject(projectName, message);
    if (Main.Push) return Main.Push({ projectName, message });
    throw new Error('No Push binding found in go/main/App');
}

// ---- PULL ----
export async function listRemoteProjects() {
    if (Main.listRemoteProjects) return Main.listRemoteProjects();
    if (Main.ListProjects) return Main.ListProjects();
    throw new Error('No ListRemoteProjects/ListProjects binding found in go/main/App');
}
export async function getPullStatus(projectName) {
  if (Main.GetPullStatus) return Main.GetPullStatus(projectName);
  // fallback if you don’t expose it yet
  return { localNewer: false };
}
export async function getCommitHistory(projectName, limit = 5) {
  if (Main.GetCommitHistory) return Main.GetCommitHistory(projectName, limit);
  return [];
}
export async function pullProject(projectName, { allowDelete = false, commitId = '' } = {}) {
  // Try common signatures
  if (Main.PullProject) {
    try { return await Main.PullProject(projectName, commitId, allowDelete); } catch {}
    try { return await Main.PullProject({ projectName, commitId, allowDelete }); } catch {}
  }
  if (Main.PullHead) return Main.PullHead(projectName, allowDelete);
  throw new Error('No PullProject/PullHead binding found in go/main/App');
}