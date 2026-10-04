// The app's version, and how it compares with the daemon's, mirroring the
// Go package internal/version so the desktop and the CLI decide alike
// whether to replace a daemon (ADR-0029 §4).

use std::cmp::Ordering;

/// DEV is the version of a build no release made.
pub const DEV: &str = "dev";

/// app_version is this app's version: the release's, from Cargo.toml, which
/// the release workflow checks against the tag. A debug build, as `tauri
/// dev` makes, says DEV, as `make dev` does for the daemon.
pub fn app_version() -> &'static str {
    if cfg!(debug_assertions) {
        DEV
    } else {
        env!("CARGO_PKG_VERSION")
    }
}

/// compare orders two versions as semantic versions. Either one being DEV,
/// or not a semantic version, compares as equal, so a development build
/// works against any other, and nothing is stopped on a version it cannot
/// read.
pub fn compare(a: &str, b: &str) -> Ordering {
    match (parse(a), parse(b)) {
        (Some(va), Some(vb)) => va
            .core
            .cmp(&vb.core)
            .then_with(|| compare_prerelease(&va.prerelease, &vb.prerelease)),
        _ => Ordering::Equal,
    }
}

struct Semver<'a> {
    core: [u64; 3],
    prerelease: Vec<&'a str>,
}

fn parse(s: &str) -> Option<Semver<'_>> {
    let s = s.strip_prefix('v').unwrap_or(s);
    let s = s.split_once('+').map_or(s, |(before, _)| before);
    let (core, prerelease) = match s.split_once('-') {
        Some((core, pre)) => {
            let ids: Vec<&str> = pre.split('.').collect();
            if ids.iter().any(|id| id.is_empty()) {
                return None;
            }
            (core, ids)
        }
        None => (s, Vec::new()),
    };
    let parts: Vec<&str> = core.split('.').collect();
    if parts.len() != 3 {
        return None;
    }
    let mut numbers = [0u64; 3];
    for (i, part) in parts.iter().enumerate() {
        if !is_numeric(part) || (part.len() > 1 && part.starts_with('0')) {
            return None;
        }
        numbers[i] = part.parse().ok()?;
    }
    Some(Semver {
        core: numbers,
        prerelease,
    })
}

fn compare_prerelease(a: &[&str], b: &[&str]) -> Ordering {
    match (a.is_empty(), b.is_empty()) {
        (true, true) => return Ordering::Equal,
        (true, false) => return Ordering::Greater,
        (false, true) => return Ordering::Less,
        _ => {}
    }
    for (x, y) in a.iter().zip(b.iter()) {
        if x == y {
            continue;
        }
        return match (is_numeric(x), is_numeric(y)) {
            (true, true) => x
                .parse::<u64>()
                .unwrap_or(0)
                .cmp(&y.parse::<u64>().unwrap_or(0)),
            (true, false) => Ordering::Less,
            (false, true) => Ordering::Greater,
            (false, false) => x.cmp(y),
        };
    }
    a.len().cmp(&b.len())
}

fn is_numeric(s: &str) -> bool {
    !s.is_empty() && s.bytes().all(|b| b.is_ascii_digit())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn orders_semantic_versions() {
        let cases = [
            ("1.2.3", "1.2.3", Ordering::Equal),
            ("1.2.3", "1.2.4", Ordering::Less),
            ("1.10.0", "1.9.0", Ordering::Greater),
            ("2.0.0", "1.99.99", Ordering::Greater),
            ("v1.2.3", "1.2.3", Ordering::Equal),
            ("1.2.3+build.5", "1.2.3", Ordering::Equal),
            ("1.0.0-alpha", "1.0.0", Ordering::Less),
            ("1.0.0", "1.0.0-rc.1", Ordering::Greater),
            ("1.0.0-alpha", "1.0.0-alpha.1", Ordering::Less),
            ("1.0.0-alpha.1", "1.0.0-alpha.beta", Ordering::Less),
            ("1.0.0-alpha.beta", "1.0.0-beta", Ordering::Less),
            ("1.0.0-beta.2", "1.0.0-beta.11", Ordering::Less),
            ("1.0.0-rc.1", "1.0.0-beta.11", Ordering::Greater),
        ];
        for (a, b, want) in cases {
            assert_eq!(compare(a, b), want, "{a} against {b}");
            assert_eq!(compare(b, a), want.reverse(), "{b} against {a}");
        }
    }

    #[test]
    fn dev_or_unreadable_is_equal() {
        for other in [
            "dev",
            "",
            "1.2",
            "1.2.3.4",
            "01.2.3",
            "1.2.x",
            "1.2.3-",
            "1.2.3-a..b",
        ] {
            assert_eq!(compare(other, "9.9.9"), Ordering::Equal, "{other:?}");
            assert_eq!(compare("0.0.1", other), Ordering::Equal, "{other:?}");
        }
    }
}
