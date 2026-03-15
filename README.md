<!--
Copyright 2026 Stanislav Senotrusov

This work is dual-licensed under the Apache License, Version 2.0
and the MIT License. Refer to the LICENSE file in the top-level directory
for the full license terms.

SPDX-License-Identifier: Apache-2.0 OR MIT
-->

# envscope

`envscope` is a static environment manager for Bash. It allows you to define directory-specific environment variables in a single, centralized configuration file.

Unlike `direnv`, which looks for `.envrc` files in every directory you visit, `envscope` uses a "compiled" approach. It parses your global configuration and generates highly optimized Bash code that hooks into your prompt. This keeps your project folders clean of hidden configuration files.

## Features

- **Centralized Config:** Manage all directory rules in `~/.config/envscope/main.conf`.
- **Fast:** Once loaded, it runs as pure Bash logic. No external binaries are called when changing directories.
- **Hierarchical:** Supports nested path matching (deepest directory wins).
- **Robust:** Compatible with `set -euo pipefail`.
- **Prepend Support:** Easily prepend to `PATH` or other variables using the `+` prefix.
- **Override Respect:** If you manually `export` a variable while inside a managed zone, `envscope` detects the manual change and will not overwrite it when you leave.

## Installation

1. **Build the binary:**

   ```bash
   just build
   just install
   ```

2. **Initialize the config:**
   Create the directory and configuration file:

   ```bash
   mkdir -p ~/.config/envscope
   touch ~/.config/envscope/main.conf
   ```

3. **Add to your shell:**
   Add the following line to the end of your `~/.bashrc`:

   ```bash
   eval "$(envscope hook bash)"
   ```

## Configuration Format

The configuration file is located at `~/.config/envscope/main.conf`.

- **Paths:** Start at the beginning of the line. `~` is automatically expanded to your home directory.
- **Variables:** Must be indented with at least one space or tab.
- **Prepending:** Use `+VAR=value` to prepend to an existing variable. If the variable is `PATH`, it automatically handles the `:` separator.
- **Dynamic Values:** You can use Bash command substitution like `$(command)`.

### Example `main.conf`

```text
~/projects/work
  PGDATABASE=work_db
  API_KEY=secret_token

~/projects/work/microservice-a
  PGDATABASE=service_a_db
  +PATH=~/projects/work/microservice-a/bin

~/sandbox
  TEMP_ENV=true
  API_KEY=$(passage show my/secrets)
```

## How it works

1. **Startup:** When you open a new shell, `envscope hook bash` runs. It reads your `main.conf` and generates a series of Bash functions and a `case` statement containing all your managed paths.
2. **Tracking:** Every time your prompt is displayed (after a `cd` or a command), the `__envscope_hook` function checks your current `$PWD` against the generated `case` statement.
3. **State Management:**
   - When entering a zone, it saves the "outer" value of any variable it is about to change.
   - When moving between nested zones, it restores the outer value before applying the new zone's variables to ensure a clean state.
   - When leaving all zones, it restores variables to their original values (or unsets them if they didn't exist).
4. **Safety:** If the current value of a variable does not match the value `envscope` last set, the tool assumes you have manually changed it and refuses to touch it, preserving your manual overrides.

## License

This work is dual-licensed under the Apache License, Version 2.0
and the MIT License. Refer to the [LICENSE](LICENSE) file in the top-level
directory for the full license terms.

## Get involved

See the [CONTRIBUTING](CONTRIBUTING.md) file for guidelines
on how to contribute, and the [CONTRIBUTORS](CONTRIBUTORS.md)
file for a list of contributors.
