<script>
    export let commits = []; // [{id,message,timestamp}, ...]
    export let initial = 5;
    let show = initial;
    function more() { show += initial; }
    function fmtDate(ts) {
        const d = new Date(ts < 2e12 ? ts * 1000 : ts)
    }
</script>