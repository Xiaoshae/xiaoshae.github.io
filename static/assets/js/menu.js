const menu = document.getElementById('site-nav');
const toggle = document.querySelector('.nav-toggle');
const desktop = matchMedia('(min-width: 900px)');

function setOpen(open) {
  const expanded = !desktop.matches && open;
  toggle.setAttribute('aria-expanded', String(expanded));
  toggle.hidden = desktop.matches;
  menu.classList.toggle('opened', expanded);
  menu.inert = !desktop.matches && !expanded;
}

setOpen(false);
document.documentElement.classList.add('js');
toggle.addEventListener('click', () => setOpen(toggle.getAttribute('aria-expanded') !== 'true'));
desktop.addEventListener('change', () => setOpen(false));
