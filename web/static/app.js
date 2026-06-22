document.addEventListener('keydown', (event) => {
  const mod = event.metaKey || event.ctrlKey;
  if (mod && event.key.toLowerCase() === 'k') {
    event.preventDefault();
    document.querySelector('#intake select')?.focus();
  }
  if (event.shiftKey && event.key.toLowerCase() === 'a') {
    event.preventDefault();
    document.querySelector('#audit')?.scrollIntoView({behavior: 'smooth', block: 'start'});
  }
});
