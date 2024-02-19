# Loadbalancer.org Feedback Agent: The Next Generation

This is the new Loadbalancer.org Feedback Agent v5 -- to boldly go where no Feedback Agent has gone before. 

The Loadbalancer.org Feedback Agent v5 is cross-platform and concurrent, written in Go, with user-configurable multiple services, non-blocking feedback responses and an advanced stats model. Services hosted by the agent are fully configurable via a JSON configuration file. The binary for the agent is self-installing in that it creates its own log and configuration directories, as well as a default configuration file, where these are missing. It currently works on most POSIX platforms (Linux, NetBSD, Mac OS X/Darwin, etc.). There is a Windows service wrapper currently in development, and an accompanying configuration tool which will be available for both UNIX and Windows environments (in varying CLI and GUI builds).

**Important Note:** This project is currently in an alpha release state and is not suitable for production release at this time.
**Please do NOT share this application with customers at this time or release this code publicly until QA has been completed.**

# Authors
- Developer: Nicholas Turnbull <nicholas.turnbull@loadbalancer.org>

# Release Notes, Known Issues and To Do

## v5.1.6-alpha (2024-02-16)
- This is a very early alpha release for initial testing internally within Loadbalancer.org.
- Only the POSIX platform target (Linux, FreeBSD, NetBSD, OpenBSD and Darwin) is supported in this release as further work needs to be done on the Windows platform target.
- There are loads of places in this code which require general cleanup which I (NT) am fully aware of - please pardon the temporary issues with this.
- There is a lack of validation on the JSON data fields for service ports, paths, names, etc. Whilst these will result in handled errors, the result will not be particularly graceful.
- TCP mode feedback is currently removed from the Feedback Responder service due to an issue with ldirectord hanging until a TCP FIN occurs on the connection.
- The Custom Script metric will currently result in an error if a non-zero exit status is returned under both Linux and Windows.
- Refactoring: The CreateMonitor() and CreateResponder() functions within FeedbackAgent (core/feedback.go) need to be moved into FeedbackResponder and SystemMonitor respectively as constructors, since that is where they more properly belong.
- The platform_windows.go file (containing the system hooks for Windows environments) is currently missing as this needs to be reworked.
- The binary will compile and run under both x86-64 and arm64 target architectures without unexpected issues. However, compilation fails on x86-32 system targets due to the reliance on int64/float64 throughout the code. I believe this may well be a sensible "won't fix" aspect of the v3 Feedback Agent, but we first need to identify if there is a need for a 32-bit compatible version before putting any work into this.
- ARM Cortex-X2, A710, A510: The gopsutil library used in this project does not report CPU utilisation meaningfully when running on mixed-core ARM Cortex-A mobile processors that contain "efficiency" cores alongside faster "prime" and "performance" cores. This is likely because the percentage utilisation does not take into account the difference in clock speed and therefore it doesn't correctly reflect resource utilisation. This causes the reported CPU usage to be consistently low irrespective of system load. The "android" build target is therefore not included in platform_posix.go at this time. (Discovered on Qualcomm Snapdragon 8+ Gen 1 under Android.)
- 2024-02-19 14:29: Version bumped from v3.x to v5.x to avoid collision with the existing Feedback Agents.