"""Read selected login classes with the installed Android SDK dexdump.

No APK is run, installed, patched, or uploaded. Output stays in ignored evidence.
"""
import json
import re
import struct
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ARTIFACTS = ROOT / "work/pangguai-login/evidence/apk"
DEXDUMP = Path("C:/Users/84272/AppData/Local/Android/Sdk/build-tools/36.0.0/dexdump.exe")


def class_names(raw):
    string_count, string_offset, type_count, type_offset = struct.unpack_from("<IIII", raw, 56)
    strings = []
    for i in range(string_count):
        position = struct.unpack_from("<I", raw, string_offset + i * 4)[0]
        while raw[position] & 128:
            position += 1
        position += 1
        strings.append(raw[position:raw.index(0, position)].decode("utf-8", "replace"))
    types = [strings[struct.unpack_from("<I", raw, type_offset + i * 4)[0]] for i in range(type_count)]
    count, offset = struct.unpack_from("<II", raw, 96)
    return [types[struct.unpack_from("<I", raw, offset + i * 32)[0]] for i in range(count)]


def main():
    selected = json.loads((ARTIFACTS / "selected-dex.json").read_text())
    for entry in selected:
        dex = ARTIFACTS / entry["dex"]
        names = class_names(dex.read_bytes())
        wanted = {i for i, name in enumerate(names) if name in entry["classes"]}
        output = dex.with_suffix(".login.txt")
        count = 0
        with output.open("w", encoding="utf-8") as destination:
            with subprocess.Popen([str(DEXDUMP), "-a", "-d", str(dex)], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8", errors="replace", creationflags=subprocess.CREATE_NO_WINDOW) as process:
                keep = False
                for line in process.stdout:
                    match = re.match(r"Class #(\d+)", line)
                    if match:
                        keep = int(match.group(1)) in wanted
                    if keep:
                        destination.write(line)
                        count += 1
                diagnostics = process.stderr.read()
                if process.wait() != 0:
                    raise RuntimeError(diagnostics[:2000])
        print(json.dumps({"dex": entry["dex"], "selected_classes": len(wanted), "saved_lines": count, "output": str(output.relative_to(ROOT))}))


if __name__ == "__main__":
    main()
