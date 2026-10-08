export function chooseElement(doc: Document, onSelect: (id: string) => void): () => void {
  let hovered: HTMLElement | undefined;
  let previous = '';
  // DOM nodes crossing extension/page execution worlds are wrapped by the browser.
  // Avoid realm-sensitive `instanceof Element/HTMLElement` checks on event targets.
  const elementTarget = (target: EventTarget | null): HTMLElement | null => {
    if (!target || typeof target !== 'object' || !('nodeType' in target)) return null;
    const node = target as Node;
    const element = node.nodeType === Node.ELEMENT_NODE ? node as Element : node.parentElement;
    return element && typeof element.closest === 'function' ? element as HTMLElement : null;
  };
  const hover = (event: MouseEvent) => {
    const target = elementTarget(event.target)?.closest<HTMLElement>('[id]') ?? null;
    if (hovered && hovered !== target) hovered.style.outline = previous;
    hovered = target ?? undefined;
    if (hovered) { previous = hovered.style.outline; hovered.style.outline = '3px solid #3677db'; }
  };
  const click = (event: MouseEvent) => {
    const clickedElement = elementTarget(event.target);
    if (clickedElement?.closest('[data-legacylens-ui]')) return;
    const target = clickedElement?.closest<HTMLElement>('[id]') ?? null;
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
