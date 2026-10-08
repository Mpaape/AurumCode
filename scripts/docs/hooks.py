"""MkDocs hook: keep docs/README.md as a normal page next to docs/index.md.

MkDocs drops README.md from the build whenever index.md exists (it treats the
two as the same home page), which breaks the many relative links to
../README.md in the specs. README.md is the repository entry point, so this
hook re-adds it as its own page (served at /README/).
"""
import fnmatch
import os
import re

from mkdocs.structure.files import File, InclusionLevel


def on_files(files, config):
    """README.md is listed in exclude_docs (so MkDocs does not warn about the
    index.md conflict); replace the excluded entry by an included one.
    MkDocs maps README.md to index.html, so the destination is explicit."""
    old = files.get_file_from_path("README.md")
    if old is not None:
        files.remove(old)
    files.append(File(
        "README.md", config["docs_dir"], config["site_dir"],
        config["use_directory_urls"],
        dest_uri="README/index.html",
        inclusion=InclusionLevel.INCLUDED,
    ))
    return files


def _natural(name):
    return [int(p) if p.isdigit() else p for p in re.split(r"(\d+)", name)]


def _section(config, folder, pattern, index):
    """Nav entries for docs/<folder>: <index> first (when present), then every
    file matching <pattern> in natural order."""
    base = os.path.join(config["docs_dir"], folder)
    if not os.path.isdir(base):
        return []
    items = []
    if os.path.isfile(os.path.join(base, index)):
        items.append(f"{folder}/{index}")
    names = sorted((n for n in os.listdir(base) if fnmatch.fnmatch(n, pattern) and n != index),
                   key=_natural)
    items.extend(f"{folder}/{n}" for n in names)
    return items


def on_config(config):
    """Inject the generated sections into the nav, before MkDocs validates
    pages: Tutoriais (docs/tutorials/README.md, then *.md) when the folder
    exists, and Specs (docs/specs/README.md, then AUR-*.md). Nothing to
    maintain by hand when a card adds a spec or a tutorial.
    tests/acceptance/AUR-560.sh reimplements this rule in bash."""
    nav = list(config["nav"] or [])
    tutorials = _section(config, "tutorials", "*.md", "README.md")
    if tutorials:
        # Right after Início: the tutorials are where a new reader goes next.
        nav.insert(min(1, len(nav)), {"Tutoriais": tutorials})
    specs = _section(config, "specs", "AUR-*.md", "README.md")
    if specs:
        nav.append({"Specs": specs})
    # External links (the GitHub tab) stay the last tabs.
    def external(entry):
        return isinstance(entry, dict) and any(isinstance(v, str) and v.startswith("https://") for v in entry.values())
    config["nav"] = [e for e in nav if not external(e)] + [e for e in nav if external(e)]
    return config
