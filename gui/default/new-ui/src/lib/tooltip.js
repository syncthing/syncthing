// Bootstrap-style tooltip Svelte action
// Usage: <element use:tooltip={'Tooltip text'}>
// Creates a styled tooltip on hover, similar to Bootstrap's data-toggle="tooltip"

// Global cleanup: remove any orphaned tooltips on click anywhere
if (typeof window !== 'undefined' && !window._tooltipGlobalCleanup) {
  window._tooltipGlobalCleanup = true;
  document.addEventListener('click', () => {
    document.querySelectorAll('.tooltip.in').forEach(el => el.remove());
  }, true);
}

export function tooltip(node, text) {
  let tooltipEl = null;
  let currentText = text;

  function show() {
    if (!currentText) return;

    tooltipEl = document.createElement('div');
    tooltipEl.className = 'tooltip top in';
    tooltipEl.setAttribute('role', 'tooltip');
    tooltipEl.innerHTML = `<div class="tooltip-arrow"></div><div class="tooltip-inner">${escapeHtml(currentText)}</div>`;
    document.body.appendChild(tooltipEl);

    position();
  }

  function position() {
    if (!tooltipEl) return;
    const rect = node.getBoundingClientRect();
    const ttRect = tooltipEl.getBoundingClientRect();

    let top = rect.top - ttRect.height + window.scrollY;
    let left = rect.left + (rect.width - ttRect.width) / 2 + window.scrollX;

    // Flip to bottom if too close to top
    if (top < window.scrollY + 5) {
      tooltipEl.className = 'tooltip bottom in';
      top = rect.bottom + window.scrollY;
    }

    // Constrain horizontal
    if (left < 5) left = 5;
    if (left + ttRect.width > window.innerWidth - 5) {
      left = window.innerWidth - ttRect.width - 5;
    }

    tooltipEl.style.top = top + 'px';
    tooltipEl.style.left = left + 'px';
  }

  function hide() {
    if (tooltipEl) {
      tooltipEl.remove();
      tooltipEl = null;
    }
  }

  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  node.addEventListener('mouseenter', show);
  node.addEventListener('mouseleave', hide);
  node.addEventListener('click', hide);
  node.addEventListener('focus', show);
  node.addEventListener('blur', hide);

  return {
    update(newText) {
      currentText = newText;
      if (tooltipEl) {
        const inner = tooltipEl.querySelector('.tooltip-inner');
        if (inner) inner.textContent = newText;
        position();
      }
    },
    destroy() {
      hide();
      node.removeEventListener('mouseenter', show);
      node.removeEventListener('mouseleave', hide);
      node.removeEventListener('click', hide);
      node.removeEventListener('focus', show);
      node.removeEventListener('blur', hide);
    }
  };
}
