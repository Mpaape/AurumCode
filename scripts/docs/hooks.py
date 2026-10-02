"""MkDocs hook: keep docs/README.md as a normal page next to docs/index.md.

MkDocs drops README.md from the build whenever index.md exists (it treats the
two as the same home page), which breaks the many relative links to
../README.md in the specs. README.md is the repository entry point, so this
hook re-adds it as its own page (served at /README/).
"""
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
