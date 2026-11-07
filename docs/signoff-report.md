# Release sign-off report

A **sign-off report** is a one-page summary of a collection's state in one environment, written for the moment a
release is handed from QA to whoever approves it: is it ready, what was tested, what regressed, what the security
checks found, what got slower, and who approved the baselines the results are compared against. It ends with signature
lines so it can be printed, signed and filed for an audit.

Find it in a collection's **⋯** menu → **Sign-off report…**.

| Section              | What it shows                                                                                          |
| -------------------- | ------------------------------------------------------------------------------------------------------ |
| **Verdict**          | **Ready for release** or **Not ready**, with every reason (failed requests, regressions, failed security checks, no run) |
| **Test run**         | The newest collection run in the environment: when, passed / failed / errors, how many of the collection's requests it covered, and the failures with their messages (including [collection rules](collection-rules.md)) |
| **Regressions**      | Requests whose response differs from the approved baseline, with the first differences                 |
| **Security checks**  | The newest [race test](race-test.md) of each request and the newest [permission matrix](permission-matrix.md) run in the environment, with their verdicts; *not run* when there is none |
| **Performance**      | Requests whose latest response was [slower than usual](performance.md), or whose median moved by 20 % or more |
| **Baselines**        | For every request: the baseline it is compared against (whole response, selected fields or structure), when it was approved and **by whom**, or *no baseline* |
| **Coverage**         | Requests without tests, without a baseline, and requests left out of the run                           |
| **Sign-off**         | Release, environment, prepared by, notes, and blank *Approved by / Date / Signature* lines             |

The report is built from what is already stored on this computer (history, baselines, response times). It sends no
request; run the collection (and the security checks) first.

## Creating a report

1. **⋯ → Sign-off report…**. Fill in:
   - **Release**: the version or name being signed off, for example `v2.3.0` (required).
   - **Environment**: the environment the release was tested in (defaults to the selected one).
   - **Prepared by**: your name. It defaults to **Settings → Your name**.
   - **Notes**: anything the approver should know (optional, plain text).
2. Press **Preview**. The report opens inside the app exactly as it will be saved or printed.
3. **Save HTML…** writes one self-contained HTML file (no external resources, opens in any browser).
   **Print / Save as PDF** opens the system print dialog; choose *Save as PDF* (Windows) or *Print to File → PDF*
   (Linux) to get a PDF. The layout is designed for A4 and Letter paper.

## When is a release "ready"?

**Ready for release** needs all of these; otherwise the report says **Not ready** and lists each reason:

- there is a collection run in this environment;
- that run passed: no failed or errored requests and no failed collection rules (a cancelled run is not ready);
- no request in it differs from its baseline;
- the newest race test of every request that has one, and the newest permission matrix run, passed.

Missing security checks, missing baselines, untested requests and slower responses do not block the verdict: they are
listed under **Coverage** and **Performance** for the approver to judge. The report does not decide for the
approver; it makes sure nothing is overlooked.

## Who approved a baseline

Baselines now record **Approved by**: the name entered in the **Approve as baseline…** dialog, which defaults to
**Settings → Your name**. Baselines approved before this version show *—*. The app has no accounts, so the name is
whatever the person approving typed; the report labels it as entered, not verified.

## Privacy and safety

- Everything comes from stored, already-redacted results, and is redacted once more with the current secret values.
  Response bodies are not included, only failure messages and differences (as in the [HTML run report](cli.md)).
- The HTML contains no scripts. The preview is shown in a sandbox without scripts.
- Nothing is saved until you choose **Save HTML…** or print.

## Try it

With the race-test demo API running (`python3 docs/demo/security_demo_api.py`, see [race-test.md](race-test.md)),
use the `Rules demo` collection from [collection-rules.md](collection-rules.md):

1. **Settings → Your name**: `Dana (QA)`. Send `Balance before`, **Approve as baseline…** (the dialog shows
   *Approved by: Dana (QA)*).
2. **Run collection**, then **⋯ → Sign-off report…**, release `v1.0.0`, **Preview**.

The verdict is **Ready for release**. *Baselines* lists `Balance before` approved by *Dana (QA)*, and *Coverage* lists
`Withdraw 10` and `Balance after` as having no baseline. Run the collection again: the balance now starts at 90, so
`Balance before` differs from its baseline, and a new preview says **Not ready**: *1 request failed* and
*1 regression from its baseline*.

## Limits

- One collection and one environment per report. Compare releases across environments with
  [environment comparison](environment-comparison.md).
- The newest run is used even if it was a folder run; *Coverage* then lists the requests it left out.
- PDF output goes through the system print dialog; there is no built-in PDF writer.
