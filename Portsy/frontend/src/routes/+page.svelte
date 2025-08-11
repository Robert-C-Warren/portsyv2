<script>
  import { ScanProjects } from '../../wailsjs/go/main/App';

  /** @typedef {{ id:string; message:string; timestamp:number }} CommitMeta */
  /** @typedef {{ name:string; path:string; alsFile:string; hasPortsy:boolean; lastCommit?: CommitMeta|null }} AbletonProject */

  /** @type {AbletonProject[]} */
  let projects = [];
  let rootPath = '';

  async function scan() {
    try {
      projects = await ScanProjects(rootPath);
    } catch (e) {
      console.error('ScanProjects failed:', e);
    }
  }
</script>

<input type="text" bind:value={rootPath} placeholder="Enter Ableton Projects Root" />
<button on:click={scan}>Scan Projects</button>

<ul>
  {#each projects as p}
    <li>
      <b>{p.name}</b> {p.hasPortsy ? "✅" : "❌"}<br />
      .als: {p.alsFile}
    </li>
  {/each}
</ul>
