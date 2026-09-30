import { readdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const publicDirectory = fileURLToPath(new URL('../public/skills/', import.meta.url))
const skillDirectory = join(publicDirectory, 'xunshu-api')

// Store entries without compression so packaging needs only Node's standard library.
const crcTable = Array.from({ length: 256 }, (_, value) => {
  for (let bit = 0; bit < 8; bit++) value = value & 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1
  return value >>> 0
})
function crc32(bytes) {
  let crc = 0xffffffff
  for (const byte of bytes) crc = crcTable[(crc ^ byte) & 0xff] ^ (crc >>> 8)
  return (crc ^ 0xffffffff) >>> 0
}

async function sourceFiles(directory) {
  const files = []
  const entries = await readdir(directory, { withFileTypes: true })
  entries.sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0)
  for (const entry of entries) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) files.push(...await sourceFiles(path))
    else if (entry.isFile()) files.push(path)
    else throw new Error(`Skill assets must be regular files: ${path}`)
  }
  return files
}

const localEntries = []
const centralEntries = []
let offset = 0
for (const path of await sourceFiles(skillDirectory)) {
  const name = Buffer.from(relative(dirname(skillDirectory), path).split(sep).join('/'), 'utf8')
  const data = await readFile(path)
  const crc = crc32(data)
  const local = Buffer.alloc(30)
  local.writeUInt32LE(0x04034b50, 0)
  local.writeUInt16LE(20, 4)
  local.writeUInt16LE(0x0800, 6) // UTF-8 filenames
  local.writeUInt16LE(33, 12) // Fixed date: 1980-01-01, 00:00:00
  local.writeUInt32LE(crc, 14)
  local.writeUInt32LE(data.length, 18)
  local.writeUInt32LE(data.length, 22)
  local.writeUInt16LE(name.length, 26)
  const central = Buffer.alloc(46)
  central.writeUInt32LE(0x02014b50, 0)
  central.writeUInt16LE(0x0314, 4) // Unix, ZIP 2.0
  central.writeUInt16LE(20, 6)
  central.writeUInt16LE(0x0800, 8)
  central.writeUInt16LE(33, 14)
  central.writeUInt32LE(crc, 16)
  central.writeUInt32LE(data.length, 20)
  central.writeUInt32LE(data.length, 24)
  central.writeUInt16LE(name.length, 28)
  central.writeUInt32LE(0o100644 * 65536, 38)
  central.writeUInt32LE(offset, 42)
  localEntries.push(local, name, data)
  centralEntries.push(central, name)
  offset += local.length + name.length + data.length
}
const centralDirectory = Buffer.concat(centralEntries)
const ending = Buffer.alloc(22)
ending.writeUInt32LE(0x06054b50, 0)
ending.writeUInt16LE(localEntries.length / 3, 8)
ending.writeUInt16LE(localEntries.length / 3, 10)
ending.writeUInt32LE(centralDirectory.length, 12)
ending.writeUInt32LE(offset, 16)
const archive = Buffer.concat([...localEntries, centralDirectory, ending])
await writeFile(join(publicDirectory, 'xunshu-api.zip'), archive)

const embeddedArchive = archive.toString('base64').match(/.{1,100}/g).map(line => `    "${line}"`).join('\n')
const installer = `#!/usr/bin/env python3
"""Install the bundled Xunshu read-only skill; no network requests or credentials."""

import argparse
import base64
import io
import os
from pathlib import Path, PurePosixPath
import shutil
import sys
import tempfile
import zipfile

ARCHIVE_BASE64 = (
${embeddedArchive}
)
SKILL_NAME = "xunshu-api"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agent", choices=("codex", "claude"), default="codex")
    parser.add_argument("--dir", type=Path, help="Custom parent directory for installed skills")
    args = parser.parse_args()
    if args.dir is not None:
        parent = args.dir.expanduser().absolute()
    elif args.agent == "codex":
        codex_directory = os.environ.get("CODEX_HOME")
        parent = (Path(codex_directory).expanduser() if codex_directory else Path.home() / ".codex") / "skills"
    else:
        parent = Path.home() / ".claude" / "skills"
    target = parent / SKILL_NAME
    lock = parent / ("." + SKILL_NAME + ".install-lock")
    staging = None
    locked = False
    try:
        parent.mkdir(parents=True, exist_ok=True)
        # Serialize cooperating installers, and never reuse an existing skill directory.
        try:
            lock.mkdir()
            locked = True
        except FileExistsError:
            raise ValueError("Another installation is in progress (lock: " + str(lock) + ")")
        if os.path.lexists(target):
            raise ValueError("Already exists; nothing was changed: " + str(target))
        staging = Path(tempfile.mkdtemp(prefix=".xunshu-api-stage-", dir=parent))
        with zipfile.ZipFile(io.BytesIO(base64.b64decode(ARCHIVE_BASE64))) as archive:
            for info in archive.infolist():
                path = PurePosixPath(info.filename)
                if (path.is_absolute() or ".." in path.parts or len(path.parts) < 2
                        or path.parts[0] != SKILL_NAME or info.is_dir()
                        or (info.external_attr >> 16) & 0o170000 != 0o100000):
                    raise ValueError("Invalid bundled skill path")
                destination = staging.joinpath(*path.parts)
                destination.parent.mkdir(parents=True, exist_ok=True)
                with destination.open("xb") as output:
                    output.write(archive.read(info))
        if os.path.lexists(target):
            raise ValueError("Already exists; nothing was changed: " + str(target))
        # Staging shares the destination filesystem; only a complete skill becomes visible.
        os.rename(staging / SKILL_NAME, target)
        print("Installed Xunshu skill: " + str(target))
        print("Set XUNSHU_URL to your instance URL and XUNSHU_API_KEY to a key from Agent access.")
        print("Reload your agent to discover the skill. No credentials or instance settings were written.")
        return 0
    except (OSError, ValueError, zipfile.BadZipFile) as error:
        print("Installation failed: " + str(error), file=sys.stderr)
        return 1
    finally:
        if staging is not None:
            shutil.rmtree(staging, ignore_errors=True)
        if locked:
            lock.rmdir()


if __name__ == "__main__":
    sys.exit(main())
`
await writeFile(join(publicDirectory, 'install.py'), installer)
console.log('Built /skills/xunshu-api.zip and /skills/install.py')
