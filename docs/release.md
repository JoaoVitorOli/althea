# Releasing

A version tag builds a release: `.github/workflows/release.yml` makes the Windows installer, the
app, the signed update for apps already installed (with deltas from the last three versions), and
a portable zip, and uploads them to a draft release. Publishing the draft makes it the latest
release, which installed copies of Althea update to.

## Once: the signing key

Updates are signed with an ed25519 key. Its public half is `updates.publicKey` in `mygo.json`,
built into every copy of Althea, which installs only updates signed with the private half.

- `go tool mygo keygen` wrote the pair to `%APPDATA%\mygo\update-keys`.
- The private key, `mygo-update.key`, is the repository secret `MYGO_UPDATER_PRIVATE_KEY`:
  `gh secret set MYGO_UPDATER_PRIVATE_KEY < "$APPDATA/mygo/update-keys/mygo-update.key"`.
- Keep a copy in a password manager. Losing it strands every installed Althea on its version;
  anyone who has it can ship code to every user. Never commit it.

## Each version

1. Raise `version` in `mygo.json`.
2. Add its section to `CHANGELOG.md`, `## 1.2.0`: it is the release's notes and what Althea shows
   when it offers the update. The workflow refuses a tag without one.
3. Commit, tag and push the tag:

   ```bash
   git tag v1.2.0 && git push origin v1.2.0
   ```

4. Review the draft release: `Althea Setup 1.2.0 amd64.exe`, `Althea-1.2.0-portable-amd64.zip`,
   `althea-1.2.0-windows-amd64.tar.gz` with its deltas, and `update-windows-amd64.json`. Install
   it on a clean user account, then publish it.

Releases tagged otherwise, as the translation engine's `bergamot-…` prereleases, are never the
latest and never offered as updates.

## Where Althea installs

The installer puts Althea in `%LOCALAPPDATA%\Programs\Althea` for the current user, with no
administrator rights, so that it can update itself. The portable zip runs from any folder and keeps
its data in a `data` folder beside it; it updates itself too where it can write.

The executable is not code-signed yet, so SmartScreen warns on the first run; signing through
SignPath comes after the first release (docs/spec.md).
