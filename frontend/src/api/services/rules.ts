// Collection rules.
import * as App from '@wails/go/main/App';

import type { Rule, RulesCheck } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const rulesService = {
  saveCollectionRules: (collectionId: string, rules: Rule[]) =>
    request<Rule[]>(Endpoint.SaveCollectionRules, () => App.SaveCollectionRules(collectionId, wire(rules))),
  checkCollectionRules: (collectionId: string, rules: Rule[]) =>
    request<RulesCheck>(Endpoint.CheckCollectionRules, () => App.CheckCollectionRules(collectionId, wire(rules))),
};
