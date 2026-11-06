import { describe, expect, it } from 'vitest';

import { axisLabel, geometry, nearestIndex, niceCeil, sparkPath } from './responseTimes';

const box = { width: 110, height: 60, left: 10, right: 0, top: 0, bottom: 10 };

describe('response time charts', () => {
  it('rounds the axis maximum to 1, 2 or 5 × 10ⁿ', () => {
    expect([0, 1, 7, 120, 180, 230, 900, 1001].map(niceCeil)).toEqual([1, 1, 10, 200, 200, 500, 1000, 2000]);
  });

  it('places points oldest first from left to right with 0 at the bottom', () => {
    const g = geometry([0, 50, 100], box);
    expect(g.xs).toEqual([10, 60, 110]);
    expect(g.ys).toEqual([50, 25, 0]);
    expect(g.path).toBe('M10,50L60,25L110,0');
    expect(g.ticks.map((t) => t.value)).toEqual([0, 50, 100]);
  });

  it('makes room for the median and limit lines', () => {
    const g = geometry([10, 20], box, [450]);
    expect(g.ticks[2].value).toBe(500);
    expect(g.y(450)).toBeCloseTo(5);
  });

  it('centres a single point', () => {
    expect(geometry([40], box).xs).toEqual([60]);
  });

  it('finds the nearest point', () => {
    expect(nearestIndex([10, 60, 110], 80)).toBe(1);
    expect(nearestIndex([10, 60, 110], 100)).toBe(2);
  });

  it('draws a sparkline only with two or more values', () => {
    expect(sparkPath([5], 40, 12)).toBe('');
    expect(sparkPath([0, 10], 40, 12)).toBe('M0,10L40,2');
    expect(sparkPath([7, 7, 7], 40, 12)).toBe('M0,10L20,10L40,10');
  });

  it('formats changes and axis labels', () => {
    expect([0, 250, 1000, 1250].map(axisLabel)).toEqual(['0 ms', '250 ms', '1 s', '1.3 s']);
  });
});
