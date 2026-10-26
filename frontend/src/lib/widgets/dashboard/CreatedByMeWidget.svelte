<script>
  import TaskListWidget from './TaskListWidget.svelte';

  let { config = {} } = $props();
</script>

<TaskListWidget {config} mode="created" />
