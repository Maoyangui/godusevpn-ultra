document.getElementById('open-panel').addEventListener('click', () => chrome.runtime.openOptionsPage());
const helpButton = document.getElementById('open-help'), tip = document.getElementById('launcher-help');
let pinned = false;
function closeHelp() { pinned = false; tip.hidden = true; }
helpButton.addEventListener('mouseenter', () => { tip.hidden = false; });
helpButton.addEventListener('mouseleave', () => { if (!pinned) tip.hidden = true; });
helpButton.addEventListener('focus', () => { tip.hidden = false; });
helpButton.addEventListener('blur', () => { if (!pinned) tip.hidden = true; });
helpButton.addEventListener('click', () => { pinned = !pinned; tip.hidden = !pinned; });
document.addEventListener('click', event => { if (event.target !== helpButton) closeHelp(); });
document.addEventListener('keydown', event => { if (event.key === 'Escape') closeHelp(); });
