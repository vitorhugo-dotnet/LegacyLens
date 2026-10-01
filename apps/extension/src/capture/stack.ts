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

/** Reduce untrusted page frames to bounded, relative JavaScript identity and explicit gaps. */
export function toStackMetadata(frames: unknown, asyncGap?: 'ASYNC_BOUNDARY'): { frameChain?: string; stackGap?: string } {
  let frameChain = '';
  try {
    if (Array.isArray(frames)) frameChain = frames.slice(0, 8).map((frame: SourceFrame) => {
      if (frame?.observation !== 'stack' || typeof frame.functionName !== 'string' || typeof frame.source !== 'string') return '';
      const name = /^[\w.$<> -]{1,64}$/.test(frame.functionName) ? frame.functionName : '';
      const file = frame.source.split('/').at(-1) ?? '';
      if (!name || !/^[a-zA-Z][\w.-]{0,63}\.js$/.test(file) || !Number.isSafeInteger(frame.line) || frame.line < 1) return '';
      return `${name}@${file}:${frame.line}`;
    }).filter(Boolean).join('>');
  } catch { frameChain = ''; }
  if (frameChain.length > 256) frameChain = '';
  const stackGap = asyncGap && !frameChain ? 'ASYNC_BOUNDARY,STACK_UNAVAILABLE'
    : asyncGap ?? (!frameChain ? 'STACK_UNAVAILABLE' : undefined);
  return { ...(frameChain ? { frameChain } : {}), ...(stackGap ? { stackGap } : {}) };
}
