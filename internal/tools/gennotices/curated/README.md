# Curated licenses

The licenses of what JobPilot ships that no package manager hands `gennotices`.
Each directory but `spdx` is one component in `THIRD-PARTY-NOTICES.txt`, and every file in it is printed.

- `go/`: Go's standard library and runtime, linked into the daemon and the CLI.
  `LICENSE` is Go's, from `go.googlesource.com/go`, and `PATENTS` is the grant every Go release carries.
- `rust/`: Rust's standard library, linked into the desktop app.
  `COPYRIGHT-library.md` is the copyright notice the Rust project publishes for the library and the crates it is built from, converted from the pinned toolchain's `share/doc/rust/COPYRIGHT-library.html`.
  `VERSION` is that toolchain's.
  A change to `desktop/src-tauri/rust-toolchain.toml` fails `make notices` until `make notices REFRESH_RUST_STD=1` writes both again, which needs the toolchain's `rust-docs` component.
- `pdfium/`: PDFium and the libraries compiled into it, which the go-pdfium module embeds as `webassembly/pdfium.wasm`.
  The files are the `licenses/` directory of [bblanchon/pdfium-binaries](https://github.com/bblanchon/pdfium-binaries)' `pdfium-wasm.tgz` for the PDFium release go-pdfium builds its WebAssembly from.
  `VERSION` is go-pdfium's version, then that PDFium release.
  A new go-pdfium fails `make notices` until they are taken again: find the PDFium release in its `.github/workflows/go.yml` (`PDFIUM_EXPERIMENTAL_VERSION`), download that release's `pdfium-wasm.tgz`, replace the files with its `licenses/`, and write both versions to `VERSION`.
- `spdx/`: the canonical text of each license a component declares without shipping a file of its own, from [spdx/license-list-data](https://github.com/spdx/license-list-data), and Apache-2.0 from apache.org.
  The MIT and BSD texts' `<year> <copyright holders>` line is filled with the component's authors.
