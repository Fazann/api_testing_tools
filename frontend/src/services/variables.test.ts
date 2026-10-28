import { describe, expect, it } from 'vitest';

import type { Environment } from '../types';
import { describeVariable, hasVariables, splitVariables } from './variables';

const env: Environment = {
  id: 'e1',
  name: 'Local',
  protected: false,
  variables: [
    { key: 'base_url', value: 'http://localhost:8080', secret: false, enabled: true },
    { key: 'token', value: 'abc', secret: true, enabled: true },
    { key: 'off', value: 'x', secret: false, enabled: false },
    { key: 'blank', value: '', secret: false, enabled: true },
  ],
};

describe('splitVariables', () => {
  it('splits text into plain parts and references with offsets', () => {
    expect(splitVariables('{{base_url}}/users/{{ id }}?x=1')).toEqual([
      { text: '{{base_url}}', name: 'base_url', start: 0, end: 12 },
      { text: '/users/', start: 12, end: 19 },
      { text: '{{ id }}', name: 'id', start: 19, end: 27 },
      { text: '?x=1', start: 27, end: 31 },
    ]);
  });

  it('ignores text that is not a valid reference', () => {
    expect(hasVariables('{{}} {{1abc}} { {x} }')).toBe(false);
    expect(hasVariables('{{$uuid}}')).toBe(true);
  });
});

describe('describeVariable', () => {
  it('mirrors how the backend resolves references', () => {
    expect(describeVariable('base_url', env).state).toBe('defined');
    expect(describeVariable('blank', env).state).toBe('empty');
    expect(describeVariable('off', env).state).toBe('disabled');
    expect(describeVariable('nope', env).state).toBe('missing');
    expect(describeVariable('$uuid', null).state).toBe('builtin');
    expect(describeVariable('base_url', null).state).toBe('missing');
  });
});
