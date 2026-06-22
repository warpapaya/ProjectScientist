const shortcuts = [
  ['1', '#admin-client-name'],
  ['2', '#admin-contact-name'],
  ['3', '#admin-service-name'],
  ['4', '#admin-reference-name'],
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
commandSearch?.addEventListener('input', (event) => filterRows(event.target.value));
commandSearch?.addEventListener('keydown', (event) => {
  if (event.key !== 'Enter') return;
  event.preventDefault();
  document.querySelector('[data-filter-table] tbody tr:not([hidden])')?.scrollIntoView({behavior: 'smooth', block: 'center'});
});

document.querySelectorAll('[data-command-target]').forEach((field) => {
  field.addEventListener('focus', () => commandSearch?.setAttribute('placeholder', `Focused ${field.getAttribute('placeholder') || field.name}`));
});
