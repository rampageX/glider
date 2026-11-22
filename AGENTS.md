# Glider fork contributor instructions

## Configuration options

- Whenever a user-facing configuration key is added, renamed, removed, or its default or unit changes in `config.go`, `subscription/config.go`, or related config registries, update `config/subscription/instance.reference.conf.example` **in the same commit**.
- State the actual source-code default, bare-number unit, optional suffixes, intended scope, and override precedence. Do not confuse deployment recommendations with application defaults.
- Keep the reference file a safe, commented-out index: do not place inline `# comments` on active configuration lines. A `#` can be meaningful inside `check=...#expect=...`, proxy fragments and other URL values, and the config parser does not strip trailing comments.
- Update `glider -h` usage, Chinese and English READMEs, change log and targeted tests whenever they are affected.
- Separate main config from rule-file-only settings. Do not leak real subscription URLs or access credentials into examples or tests.
