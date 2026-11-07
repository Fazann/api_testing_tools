// OpenAPI contract checks.
import * as App from '@wails/go/main/App';

import type { ContractCoverage, ContractInfo } from '@/types';

import { Endpoint } from '../endpoints';
import { request } from '../request';

export const contractService = {
  getContract: (collectionId: string) =>
    request<ContractInfo>(Endpoint.GetContract, () => App.GetContract(collectionId)),
  /** Opens a file dialog; null when cancelled. */
  attachContract: (collectionId: string) =>
    request<ContractInfo | null>(Endpoint.AttachContract, () => App.AttachContract(collectionId)),
  setContractOptions: (collectionId: string, enabled: boolean, allowUndocumented: boolean) =>
    request<ContractInfo>(Endpoint.SetContractOptions, () =>
      App.SetContractOptions(collectionId, enabled, allowUndocumented),
    ),
  removeContract: (collectionId: string) =>
    request<void>(Endpoint.RemoveContract, () => App.RemoveContract(collectionId)),
  getContractCoverage: (collectionId: string, envId: string) =>
    request<ContractCoverage>(Endpoint.GetContractCoverage, () => App.GetContractCoverage(collectionId, envId)),
};
