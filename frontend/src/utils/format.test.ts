import { describe, expect, it } from 'vitest';

import { formatBytes, formatChange, formatDuration, formatTime, prettyBody } from './format';

describe('format', () => {
  it('formats JSON bodies and leaves other text alone', () => {
    expect(prettyBody('{"a":1}')).toBe('{\n  "a": 1\n}');
    expect(prettyBody('<html>')).toBe('<html>');
    expect(prettyBody('{broken', 'application/json')).toBe('{broken');
  });

  it('formats sizes, durations and times', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(2048)).toBe('2.0 KB');
    expect(formatBytes(3 * 1024 * 1024)).toBe('3.0 MB');
    expect(formatDuration(999)).toBe('999 ms');
    expect(formatDuration(1500)).toBe('1.50 s');
    expect(formatTime()).toBe('—');
    expect(formatTime('not a date')).toBe('—');
  });

  it('formats signed changes', () => {
    expect([42.4, -8.2, 0.3, null].map(formatChange)).toEqual(['+42%', '−8%', '±0%', '—']);
  });
});
