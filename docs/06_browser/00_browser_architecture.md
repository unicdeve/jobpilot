# Browser Architecture

Use Go Playwright as the initial browser automation technology.

Chrome/Chromium is expected to be installed by the user.

Browser automation is post-MVP.

The browser layer is isolated behind interfaces so the rest of the application does not depend on Playwright implementation details.
