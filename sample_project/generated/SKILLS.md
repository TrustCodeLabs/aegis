# Skills

## notes_management

**Description**  
Manage note creation, lookup, update and deletion for a tenant.

**Use when**
- a caller needs to register a new note
- a workspace flow needs tenant-scoped persistent memory
- a caller wants to remove an obsolete note
- a workspace clean-up flow must delete known data
- a caller needs one specific note
- a follow-up step needs note content after listing ids
- a caller needs to browse tenant notes
- an assistant needs note identifiers before fetching details
- a caller needs to edit a note
- a workflow must correct or enrich note content

**Do not use when**
- the caller only needs to inspect an existing note
- the action should not mutate tenant data
- the caller is unsure which note should be deleted
- the action should preserve historical content
- the note id is unknown
- the action should browse across multiple notes
- the caller already knows the target note id
- the action should create or mutate state
- the note does not exist
- the action should be append-only

**Operations**
- `notes.create`
- `notes.delete`
- `notes.get`
- `notes.list`
- `notes.update`

### notes.create
- Title: Create note
- Summary: Creates a note for the current tenant and updates the note index.
- Class: mutate
- Input: object with title, content, internal, id
- Output: object with note
- Side effects:
  - writes note data to storage
  - updates the note index
- Confirmation required: no

### notes.delete
- Title: Delete note
- Summary: Deletes a note for the current tenant after explicit confirmation.
- Class: mutate
- Input: object with id
- Output: object with id, deleted
- Side effects:
  - reads note data
  - deletes note storage
  - updates the note index
- Confirmation required: yes

### notes.get
- Title: Get note
- Summary: Fetches a single note by id from the current tenant.
- Class: read
- Input: object with id
- Output: object with note
- Side effects:
  - reads note data from storage
- Confirmation required: no

### notes.list
- Title: List notes
- Summary: Lists tenant-scoped notes ordered by most recent update.
- Class: read
- Input: object
- Output: object with notes
- Side effects:
  - reads the tenant note index from storage
- Confirmation required: no

### notes.update
- Title: Update note
- Summary: Updates an existing tenant note and refreshes the note index.
- Class: mutate
- Input: object with title, content, internal, id
- Output: object with note
- Side effects:
  - reads note data from storage
  - writes updated note data
  - updates the note index
- Confirmation required: no

