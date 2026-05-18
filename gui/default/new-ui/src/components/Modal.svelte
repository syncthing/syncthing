<script>
  import { onMount, onDestroy } from 'svelte';
  import { fade } from 'svelte/transition';

  let { title = '', heading = '', status = 'default', icon = '', large = false, closeable = true, onclose, children } = $props();
  let displayTitle = $derived(title || heading);

  let modalEl = $state(null);
  let zIndex = $state(1050);
  let backdropZIndex = $state(1040);

  onMount(() => {
    document.body.classList.add('modal-open');
    // Stack modals: count existing modals and increase z-index
    const existingModals = document.querySelectorAll('.modal');
    const stackLevel = existingModals.length - 1; // -1 because this modal is already in DOM
    if (stackLevel > 0) {
      zIndex = 1050 + stackLevel * 20;
      backdropZIndex = 1040 + stackLevel * 20;
    }
    // Focus trap
    if (modalEl) {
      const focusable = modalEl.querySelector('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
      if (focusable) focusable.focus();
    }
  });

  onDestroy(() => {
    // Only remove modal-open if no other modals remain
    setTimeout(() => {
      if (!document.querySelector('.modal')) {
        document.body.classList.remove('modal-open');
      }
    }, 200);
  });

  function handleBackdrop(e) {
    if (e.target === e.currentTarget) {
      if (onclose) onclose();
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Escape' && onclose) {
      onclose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="modal" style="display: block; overflow-y: auto; z-index: {zIndex};" transition:fade={{ duration: 150 }} onclick={handleBackdrop} bind:this={modalEl}>
  <div class="modal-dialog" class:modal-lg={large}>
    <div class="modal-content">
      <div class="modal-header {status !== 'default' ? 'alert alert-' + status : ''}">
        <h4 class="modal-title">
          {#if icon}
            <div class="panel-icon"><span class={icon}></span></div>
          {/if}
          {displayTitle}
        </h4>
      </div>
      {@render children()}
    </div>
  </div>
</div>
<div class="modal-backdrop in" style="z-index: {backdropZIndex};" transition:fade={{ duration: 150 }}></div>
