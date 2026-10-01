import { describe, expect, it } from 'vitest';
import { parseStack, toStackMetadata } from './stack.ts';

describe('parseStack', () => {
  it('keeps observed first to second to fetch frames in source order without query values', () => {
    const frames = parseStack('Error\n    at fetch (https://app.example/js/app.js?token=secret:30:5)\n    at second (https://app.example/js/app.js?token=secret:20:3)\n    at first (https://app.example/js/app.js?token=secret:10:1)');
    expect(frames.filter((frame) => frame.observation === 'stack').map((frame) => frame.functionName)).toEqual(['first', 'second', 'fetch']);
    expect(JSON.stringify(frames)).not.toContain('secret');
    expect(frames.every((frame) => frame.observation === 'stack')).toBe(true);
  });

  it('marks absent or unsupported stacks as an explicit gap', () => {
    expect(parseStack('')).toEqual([{ observation: 'gap', reason: 'STACK_UNAVAILABLE' }]);
    expect(parseStack('unsupported')).toEqual([{ observation: 'gap', reason: 'STACK_UNAVAILABLE' }]);
  });

  it('retains second to fetch observed frames and an explicit async scheduling gap', () => {
    const frames = parseStack('Error\n    at fetch (https://app.example/js/app.js:30:5)\n    at second (https://app.example/js/app.js:20:3)');
    expect(frames.filter((frame) => frame.observation === 'stack').map((frame) => frame.functionName)).toEqual(['second', 'fetch']);
    expect(toStackMetadata(frames, 'ASYNC_BOUNDARY')).toEqual({ frameChain: 'second@app.js:20>fetch@app.js:30', stackGap: 'ASYNC_BOUNDARY' });
    expect(toStackMetadata([{ observation: 'gap', reason: 'STACK_UNAVAILABLE' }], 'ASYNC_BOUNDARY'))
      .toEqual({ stackGap: 'ASYNC_BOUNDARY,STACK_UNAVAILABLE' });
  });
});
