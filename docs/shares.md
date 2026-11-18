# Managing shares

Available in source after v1.0.0-rc.2; the RC2 image does not include share editing.

Open **Shares** in the main sidebar. Users with Share and Download permission can manage their own links; administrators with those permissions can manage links across users. Existing bookmarks to `/settings/shares` redirect to `/shares`. Creating a link remains available from My files.

Each row shows the target path, expiration, password-protection status and, for administrators, the owner. Copy, edit and delete actions are available. Editing preserves the public URL, target path and ownership.

Password changes offer **leave unchanged**, **set a new password**, or **remove protection**. Existing passwords and stored hashes are never displayed. A new password rotates the server-side access token, invalidating previously unlocked access on subsequent requests. Removing protection intentionally makes the link accessible without a password. Already-authorized transfers may finish; bytes previously downloaded cannot be recalled. Passwords support up to 72 UTF-8 bytes.

Expiration can be preserved, made permanent or replaced with a positive duration in seconds, minutes, hours or days. A new duration starts when the server saves it; it is not added to the old expiration. Expired links are removed and cannot be revived by editing; create a new link instead. Refresh reloads current values if another owner/admin changed a link.

## API and persistence

Authenticated `PUT /api/share/{hash}` accepts optional `password`, `expires` and `unit` fields. Omitted password/expiration fields remain unchanged. An empty password removes protection; an empty expires string makes a link permanent. Positive whole-number duration strings use the selected unit (hours by default). Unknown fields, invalid units, zero/negative/overflowing durations and overlong passwords are rejected. Supplying no changes is rejected.

```json
{"password":"A new passphrase","expires":"7","unit":"days"}
```

The response includes only `hash`, `path`, `userID`, `expire` and `hasPassword`. It never exposes the password hash or bypass token. Ownership is checked on update; ordinary users cannot modify other users' links or reassign a share's owner/path/hash. Independent password/expiration edits use an atomic read/modify/write transaction. Deleting a share concurrently cannot resurrect it, and expiration cleanup conditionally deletes only records that are still expired.
