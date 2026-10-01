export function chooseElement(doc: Document, onSelect: (id: string) => void): () => void {
  let hovered: HTMLElement | undefined;
  let previous = '';
  const hover = (event: MouseEvent) => {
    const target = event.target instanceof HTMLElement ? event.target.closest<HTMLElement>('[id]') : null;
    if (hovered && hovered !== target) hovered.style.outline = previous;
    hovered = target ?? undefined;
    if (hovered) { previous = hovered.style.outline; hovered.style.outline = '3px solid #3677db'; }
  };
  const click = (event: MouseEvent) => {
    if (event.target instanceof Element && event.target.closest('[data-legacylens-ui]')) return;
    const target = event.target instanceof HTMLElement ? event.target.closest<HTMLElement>('[id]') : null;
    if (!target) return;
    cleanup();
    onSelect(target.id);
  };
  const cleanup = () => {
    if (hovered) hovered.style.outline = previous;
    doc.removeEventListener('mouseover', hover, true);
    doc.removeEventListener('click', click, true);
  };
  doc.addEventListener('mouseover', hover, true);
  doc.addEventListener('click', click, true);
  return cleanup;
}
