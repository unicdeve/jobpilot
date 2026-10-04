import { lazy, type ComponentType, type LazyExoticComponent } from "react";

/// lazyPage is a screen exported by name from a module that is loaded the
/// first time the screen renders, so the window starts by parsing the app
/// shell rather than every screen and the libraries only some of them use
/// (the markdown renderer, for one). load is the module's dynamic import;
/// calling it again returns the module already loaded.
export function lazyPage<M extends Record<K, ComponentType>, K extends keyof M>(
  load: () => Promise<M>,
  name: K,
): LazyExoticComponent<M[K]> {
  return lazy(() => load().then((module) => ({ default: module[name] })));
}
