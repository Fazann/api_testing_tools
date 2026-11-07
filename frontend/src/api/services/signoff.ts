// Sign-off reports.
import * as App from '@wails/go/main/App';

import type { SignoffInput, SignoffResult } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const signoffService = {
  buildSignoffReport: (input: SignoffInput) =>
    request<SignoffResult>(Endpoint.BuildSignoffReport, () => App.BuildSignoffReport(wire(input))),
  /** Opens a save dialog; resolves to the chosen path, or '' when cancelled. */
  saveSignoffReport: (fileName: string, html: string) =>
    request<string>(Endpoint.SaveSignoffReport, () => App.SaveSignoffReport(fileName, html)),
};
