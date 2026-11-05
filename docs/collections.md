# Managing collections

A **collection** groups folders and requests. It also holds the settings they share: variables, auth, required
variables and scripts. Collections are listed in the **Requests** sidebar. Each one has a **▶** button that runs the
whole collection and a **⋯** menu with the actions below.

| Action | What it does |
| --- | --- |
| **New request** / **New request from cURL…** / **New folder** | Adds an item at the top level of the collection |
| **Run collection** | Runs every request in order (same as **▶**) |
| **Endpoints summary…** | Lists every request with its method, URL, folder, tests and last result |
| **Response times…** | Median, p95, latest time and change for every request; see [performance.md](performance.md) |
| **Permission matrix…** | Runs endpoints as every role; see [permission-matrix.md](permission-matrix.md) |
| **Settings & auth…** | Name, description, variables, auth, required variables and scripts |
| **Rename** | Changes the collection's name and nothing else |
| **Duplicate** | Copies the collection with its folders, requests and tests as "*name* (copy)". Baselines and history are not copied |
| **Export…** | Saves the collection to a JSON file; see [collection-format.md](collection-format.md) |
| **Delete** | Permanently deletes the collection with its folders, requests, tests and baselines. Run history is kept |

## Create

Click **＋** at the top of the sidebar (or **Create a collection** when the list is empty) and enter a name.
You can also **Import** a collection export or an OpenAPI / Swagger file.

## Rename

**⋯ → Rename** opens a dialog with the current name. Edit it and press **Rename**. If you leave the name unchanged,
nothing is saved. The sidebar is sorted by name, ignoring case, so the collection may move to a new position.

Names follow the same rules everywhere:

- spaces at the start and end are removed;
- the name must not be empty, can be at most 200 characters long, and cannot contain line breaks;
- names don't have to be unique, but give each collection its own name if you use the CLI's `--collection` (see below).

Rename changes **only the name**. Variables, auth, scripts, folders, requests, tests and baselines stay as they are.
This includes collection variables that a script changed with `pm.collectionVariables.set` during a run, even if
the sidebar hasn't refreshed since.

You can also change the name in **⋯ → Settings & auth…**. That dialog saves all collection settings together, so use
it when you change other settings at the same time.

### What a rename affects

| Where | After a rename |
| --- | --- |
| Requests, tests, baselines | Unchanged. They are linked to the collection's ID, not its name |
| **History** | Past runs keep the name the collection had when they ran. The **Collection** filter still finds them, because it filters by ID |
| **Export…** | The suggested file name uses the new name, e.g. `Wallet v2.collection.json` (characters such as `/` and `:` become `-`) |
| Files exported earlier | Unchanged. They keep the old name until you export again |
| CLI `--collection NAME` | Matches the **current** name (case-insensitive). Update scripts and cron jobs that use the old name |

If two collections have the same name, `test-tool-kit-cli run --collection` refuses to guess:

```text
2 collections are named "Wallet demo"; rename one in the app
```

Rename one of them and run the command again.

## Duplicate and delete

**Duplicate** gives the copy new IDs throughout. Baselines are **not** copied, so approve new ones in the copy
before relying on regression checks there. If the name with " (copy)" added is longer than 200 characters, it is cut to 200.

**Delete** asks for confirmation and cannot be undone. Export the collection first if you might need it again.
Runs in **History** stay readable because they keep a copy of the collection and request names.

## For developers

| Layer | Rename |
| --- | --- |
| Storage | `Store.RenameCollection(ctx, id, name)`: validates the name, then `UPDATE collections SET name, updated_at`. Returns `ErrNotFound` for an unknown ID |
| Service | `Service.RenameCollection` trims spaces from the name |
| Binding | `App.RenameCollection(id, name)` → `api.renameCollection(id, name)` |
| Store | `useWorkspace().renameCollection(id, name)` reloads the collection list |

`SaveCollection` overwrites every field. `RenameCollection` exists so a rename can't save a stale copy of the
collection's variables. Covered by `TestRenameCollectionKeepsOtherFields` in `internal/storage/sqlite_test.go`.
