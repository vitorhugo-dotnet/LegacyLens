export type SourceFrame =
  | { observation: 'stack'; functionName: string; source: string; line: number; column: number }
  | { observation: 'gap'; reason: 'STACK_UNAVAILABLE' };

/** V8/Chromium stack frames; retain only a path without URL query, hash or origin. */
export function parseStack(stack: string): SourceFrame[] {
  const frames: SourceFrame[] = [];
  for (const line of stack.split('\n').slice(1, 33)) {
    const match = /^\s*at\s+(.+?)\s+\((.+):(\d+):(\d+)\)\s*$/.exec(line);
    if (!match) continue;
    let source: string;
    try {
      const url = new URL(match[2]!);
      if (url.protocol !== 'http:' && url.protocol !== 'https:') continue;
      source = url.pathname;
    } catch { continue; }
    const functionName = match[1]!.replace(/[^\w.$<> -]/g, '').slice(0, 96);
    if (!functionName) continue;
    frames.push({ observation: 'stack', functionName, source: source.slice(0, 256), line: Number(match[3]), column: Number(match[4]) });
  }
  return frames.length ? frames.reverse() : [{ observation: 'gap', reason: 'STACK_UNAVAILABLE' }];
}
