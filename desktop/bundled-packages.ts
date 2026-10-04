import fs from "node:fs";
import path from "node:path";
import type { Plugin } from "vite";

/// bundledPackages records which npm packages the build put into the app,
/// as `name@version`, in desktop/bundled-packages.json. It is what the
/// third-party notices list for the frontend (`make notices`): exactly the
/// packages whose code ships, and none of the ones only the build runs,
/// such as Tailwind or the shadcn CLI. The file is committed, so a change
/// to what ships shows in review, and CI fails when it is out of date.
export function bundledPackages(): Plugin {
  const root = import.meta.dirname;
  return {
    name: "jobpilot:bundled-packages",
    apply: "build",
    generateBundle(_, bundle) {
      const packages = new Set<string>();
      for (const output of Object.values(bundle)) {
        if (output.type !== "chunk") continue;
        for (const id of output.moduleIds) {
          const pkg = packageOf(id);
          if (pkg) packages.add(pkg);
        }
      }
      const list = [...packages].sort();
      fs.writeFileSync(path.join(root, "bundled-packages.json"), JSON.stringify(list, null, 2) + "\n");
    },
  };
}

/// packageOf is the package a bundled module belongs to, read from the
/// package.json of the innermost node_modules directory on its path.
function packageOf(id: string): string | undefined {
  const file = id.replace(/^\0/, "").split("?")[0];
  const marker = `${path.sep}node_modules${path.sep}`;
  const at = file.lastIndexOf(marker);
  if (at < 0) return undefined;
  const rest = file.slice(at + marker.length).split(path.sep);
  const name = rest[0].startsWith("@") ? `${rest[0]}/${rest[1]}` : rest[0];
  const manifest = path.join(file.slice(0, at + marker.length), name, "package.json");
  const { version } = JSON.parse(fs.readFileSync(manifest, "utf8")) as { version: string };
  return `${name}@${version}`;
}
