const shortcuts = [
  ['1', '#admin-client-name'],
  ['2', '#admin-contact-name'],
  ['3', '#admin-service-name'],
  ['4', '#admin-reference-name'],
  ['5', '#result-entry .result-cell'],
  ['6', '#result-review input, #result-review button'],
];

function focusTarget(selector) {
  const target = document.querySelector(selector);
  if (!target) return;
  target.scrollIntoView({behavior: 'smooth', block: 'start'});
  window.setTimeout(() => target.focus(), 160);
}

function filterRows(query) {
  const needle = query.trim().toLowerCase();
  document.querySelectorAll('[data-filter-table] tbody tr').forEach((row) => {
    row.hidden = Boolean(needle) && !row.textContent.toLowerCase().includes(needle);
  });
}

document.addEventListener('keydown', (event) => {
  const mod = event.metaKey || event.ctrlKey;
  const key = event.key.toLowerCase();
  if (mod && key === 'k') {
    event.preventDefault();
    focusTarget('#admin-command-search');
    return;
  }
  if (event.shiftKey && key === 'a') {
    event.preventDefault();
    document.querySelector('#audit')?.scrollIntoView({behavior: 'smooth', block: 'start'});
    return;
  }
  const shortcut = shortcuts.find(([digit]) => event.altKey && key === digit);
  if (shortcut) {
    event.preventDefault();
    focusTarget(shortcut[1]);
  }
});

const commandSearch = document.querySelector('#admin-command-search');
const csrfToken = document.querySelector('meta[name="psc-csrf-token"]')?.content || '';
if (csrfToken) {
  document.querySelectorAll('form[method="post" i]').forEach((form) => {
    if (!form.querySelector('input[name="csrf_token"]')) {
      const field = document.createElement('input');
      field.type = 'hidden';
      field.name = 'csrf_token';
      field.value = csrfToken;
      form.prepend(field);
    }
  });
}
commandSearch?.addEventListener('input', (event) => filterRows(event.target.value));
commandSearch?.addEventListener('keydown', (event) => {
  if (event.key !== 'Enter') return;
  event.preventDefault();
  document.querySelector('[data-filter-table] tbody tr:not([hidden])')?.scrollIntoView({behavior: 'smooth', block: 'center'});
});

// The result grid keeps its autofocus marker for keyboard-first lab operators, but
// the prospect/demo landing experience should start at the SaaS cockpit instead
// of jumping halfway down the page on initial load.
window.addEventListener('load', () => {
  if (window.location.hash) return;
  window.setTimeout(() => window.scrollTo({top: 0, left: 0, behavior: 'instant'}), 0);
});

document.querySelectorAll('[data-command-target]').forEach((field) => {
  field.addEventListener('focus', () => commandSearch?.setAttribute('placeholder', `Focused ${field.getAttribute('placeholder') || field.name}`));
});

function resultGridFields() {
  return Array.from(document.querySelectorAll('[data-result-entry-grid] input.result-cell'));
}

function moveResultFocus(current, direction = 1) {
  const fields = resultGridFields().filter((field) => !field.disabled && field.offsetParent !== null);
  const index = fields.indexOf(current);
  if (index === -1) return false;
  const next = fields[index + direction];
  if (!next) return false;
  next.focus();
  if (typeof next.select === 'function') next.select();
  return true;
}

document.querySelectorAll('[data-result-entry-grid] input.result-cell').forEach((field) => {
  field.addEventListener('keydown', (event) => {
    const mod = event.metaKey || event.ctrlKey;
    if (mod && event.key === 'Enter') {
      event.preventDefault();
      field.closest('form')?.requestSubmit();
      return;
    }
    if (event.key === 'Enter') {
      event.preventDefault();
      moveResultFocus(field, event.shiftKey ? -1 : 1);
    }
  });
  field.addEventListener('focus', () => field.closest('tr')?.classList.add('active-row'));
  field.addEventListener('blur', () => field.closest('tr')?.classList.remove('active-row'));
});
