# Loadbalancer.org Feedback Agent: The Next Generation

This is the new Loadbalancer.org Feedback Agent v5 -- to boldly go where no Feedback Agent has gone before. 

The Loadbalancer.org Feedback Agent v5 is cross-platform and concurrent, written in Go, with user-configurable multiple services, non-blocking feedback responses and an advanced stats model. Services hosted by the agent are fully configurable via a JSON configuration file. The binary for the agent is self-installing in that it creates its own log and configuration directories, as well as a default configuration file, where these are missing. It currently works on most POSIX platforms (Linux, NetBSD, Mac OS X/Darwin, etc.). There is a Windows service wrapper currently in development, and an accompanying configuration tool which will be available for both UNIX and Windows environments (in varying CLI and GUI builds).

**Important Note:** This project is currently in an alpha release state and is not suitable for production release at this time.
**Please do NOT share this application with customers at this time or release this code publicly until QA has been completed.**

# Credits

## Developers
- Nicholas Turnbull (nicholas.turnbull@loadbalancer.org)

## Testers
- Andrei Grigoraş (andrei.grigoras@loadbalancer.org)
- Damian Pacuszka (damian.pacuszka@loadbalancer.org)
- Neil Stone (neil.stone@loadbalancer.org)

## QA Reviewers
- Dave Saunders (dave.saunders@loadbalancer.org)
- Andrew Zak (andrew.zak@loadbalancer.org)

# Incredibly rough initial documentation
*Please accept my sincerest apologies for the brevity of these notes, which I will update as time permits. -- NT*
- Download the most recent version of the alpha from the `binaries/` directory of this repository for the target platform and decompress it somewhere convenient.
- It does not matter where the binary is placed, as it will run from any directory. However, in order to write its logs and configuration files, it must be run with root permissions.
- The binary is designed to be dependency-free irrespective of the platform build target; all required libraries should be compiled in.
## Linux x86_64
- For testing purposes, the easiest way to see its output and to terminate it if required is to run the Agent interactively on the shell: `sudo ./lbfeedback`
- It can also be run via Upstart or simply sent to the background with `sudo nohup ./lbfeedback &` if desired.
- The service handles signals correctly and will gracefully terminate with a SIGINT, as well as restarting on SIGHUP.
- The Agent creates its own log path and log at: `/var/log/loadbalancer.org/lbfeedback/agent.log`
- The Agent creates its own JSON configuration path and default file if they do not exist: `/etc/loadbalancer.org/lbfeedback/agent-config.json`. It will create a default configuration of a CPU monitor listening on TCP port 3333 if no configuration exists.
- Please review and edit the above file to play with the configuration settings; the format should be fairly self-explanatory. An arbitrary number of multiple Monitors and Responders may be defined. The "input-monitor" JSON field tells a Responder which Monitor to get its data from.
- Supported Responder types are "http" and "tcp".
- Supported Monitor types are "cpu", "ram" and "script".
- If using the "script" monitor type in the JSON config, you will also need to set "script-name" for the monitor to the filename only (not the full path) and place this file in `/etc/loadbalancer.org/lbfeedback`. The script must output a load value between 0-100 (the inverse of the reported feedback weight from the Agent) to STDOUT, not the exit status. An exit status other than 0 will result in an error stating that the feedback script failed to run.

# Release Notes, Known Issues and To Do

## v5.1.8r2-alpha (2024-02-20)
- Linux build target: Remove accidental dependency on `glibc` by recompiling the binary using the Go-native `netgo` library instead. Checking the binary using `ldd` now reports "not a dynamic executable" as it should do. There are now no dynamic dependencies.
- Create `build.sh` script in `agent/core` to enforce this.

## v5.1.8-alpha (2024-02-20)
- Numerous bug fixes; refactoring to SystemMonitor and FeedbackResponder services.
- TCP Mode is now available again and is the default feedback mode.
- Known Issue: The JSON config needs decoupling from the object types in the code as it has ended up duplicating fields so that it ends up in a usable format.
- First Agent release ready for initial testing.
- The Configuration Tool is still in progress and will be released for alpha testing as it is available.

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