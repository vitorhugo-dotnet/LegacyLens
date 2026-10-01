import { describe, expect, it } from 'vitest';
import { parseStack } from './stack.ts';

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
});
