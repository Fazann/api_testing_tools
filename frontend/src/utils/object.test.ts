import { describe, expect, it } from 'vitest';

import { lines, mergeLines } from './object';

describe('lines', () => {
  it('trims and drops empty lines', () => {
    expect(lines(' a \n\n b\n')).toEqual(['a', 'b']);
  });
});

describe('mergeLines', () => {
  it('appends new lines once, keeping the existing order', () => {
    expect(mergeLines('b\na\n', ['a', 'c', 'c'])).toBe('b\na\nc');
    expect(mergeLines('', ['x'])).toBe('x');
  });
});
