# Loadbalancer.org Feedback Agent: The Next Generation

## Project Overview

This is the new Loadbalancer.org Feedback Agent v5 -- to boldly go where no Feedback Agent has gone before. 

The Loadbalancer.org Feedback Agent v5 is cross-platform and concurrent, written in Go, with user-configurable multiple services, non-blocking feedback responses and an advanced stats model. Services hosted by the agent are fully configurable via a JSON configuration file. The binary for the agent is self-installing in that it creates its own log and configuration directories, as well as a default configuration file, where these are missing. It currently works on most POSIX platforms (Linux, NetBSD, Mac OS X/Darwin, etc.). There is a Windows service wrapper currently in development, and an accompanying configuration tool which will be available for both UNIX and Windows environments (in varying CLI and GUI builds).

**Important Note:** This project is currently in an alpha release state and is not suitable for production release at this time.
**Please do NOT share this application with customers at this time or release this code publicly until QA has been completed.**

## Credits

### Developers
- Nicholas Turnbull (nicholas.turnbull@loadbalancer.org)

### Testers
- Andrei Grigoraş (andrei.grigoras@loadbalancer.org)
- Damian Pacuszka (damian.pacuszka@loadbalancer.org)
- Neil Stone (neil.stone@loadbalancer.org)

### QA Reviewers
- Dave Saunders (dave.saunders@loadbalancer.org)
- Andrew Zak (andrew.zak@loadbalancer.org)

## Incredibly rough initial documentation
*Please accept my sincerest apologies for the brevity of these notes, which I will update as time permits. -- NT*
- It does not matter where the binary is placed, as it will run from any directory. However, in order to write its logs and configuration files, it must be run with root permissions.
- The binary is designed to be dependency-free irrespective of the platform build target; all required libraries should be compiled in.
### Linux x86_64
- Download the most recent version of the Feedback Agent from the `binaries/` directory of this repository for the target platform. **You do not need to build the binary for yourself as it is designed to be portable and dependency free.**
- Untar the binary to a suitable location in your PATH (e.g. `/usr/bin`).
- The binary has two "personalities"; if run with the command `lbfeedback run-agent` (as `sudo`) this will start the agent itself. This can be used either for testing the agent interactively or as the appropriate shell command to place in a startup script (e.g. an init or Upstart service, or a cron job). The user will not have to do this manually as Andy's install script (via a link from the Portal) will create the appropriate startup service.
- You may wish to either use a separate terminal window so that you can view the real-time log events whilst testing with the CLI API client or send it to the background.
- When run with `lbfeedback action` this launches the binary into the CLI client personality which allows it to send API commands to the running Agent. The Agent instance itself running in the background is responsible for updating the JSON configuration file and the CLI mode of the binary merely acts as an API client. The API key is fetched from the configuration file located at /opt/lbfeedback/agent-config.json to give the CLI personality of the binary the necessary credentials to access the agent API. The CLI Client personality does not require `sudo` privileges.
- For initial MVP testing, simply check that the agent runs and creates a default configuration in /opt/lbfeedback. This should consist of:
  - A TCP mode Responder with HAProxy commands disabled listening on all IPs on port 3333. Test this using Telnet to port 3333.
  - An HTTP mode API Responder listening on 127.0.0.1 on port 3334. Test this using a Web browser.
  - Run the agent interactively in a console window as above (as sudo), and in another console window (most convenient), try the following CLI client commands (no sudo required)
  - `lbfeedback action get-feedback -name default`
  - `lbfeedback action get-config`
  - `lbfeedback action status`
  - `lbfeedback action haproxy-enable -name default` (now observe the HAProxy command in telnet and `get-feedback`)
  - `lbfeedback action haproxy-down -name default` (observe it now sends `down` in telnet)
  - `lbfeedback action haproxy-clear -name default` (observe it now sends `up` again)
  - `lbfeedback action haproxy-set-threshold -name default -threshold-value 80` (use `stress` or similar to increase CPU utilisation and observe that `down` is sent >20% CPU)
  - There are more commands available and working in the API/agent (but for which the docs don't yet exist), including:
    - Adding, editing, deleting, stopping and starting Responders and Monitors
    - Script type monitors
    - The ability to change the IP address and port of the API
    - Stopping and starting the agent from the CLI
## Release Notes, Known Issues and To Do

## v5.2.1-beta (2024-10-21)
- A massive number of changes for the Linux MVP release.
- Known issues:
- - Colours from the `logrus` library are making their way through into the logfile in `/var/log/lbfeedback` which is very annoying. Strangely, this is harder to fix than you might imagine as the option to disable colours also has the effect of disabling the formatting.
- - The error output from the CLI client mode is not very user-friendly.
- - There is no self-documentation yet in the binary; that is, a `help` command is currently missing.
- - There are almost no default parameters where these don't need to be specified.
- - The CLI personality lacks prevalidation of parameters before sending to the API.
- - A more human-friendly output than the pretty-printed JSON would be nice for the CLI result.
- - Nick has not yet documented the CLI/API - there is a tonne of available options and commands (including adding, removing, editing stuff) but this needs to be written up.

### v5.1.9-alpha (2024-03-06)
- Significant code cleanup and refactoring for the `SystemMetric` and `SystemMonitor` types.
- Improvements to JSON configuration format, including the ability to configure selected `StatisticsModel` parameters.
- Miscellaneous minor bug fixes.

### v5.1.8r2-alpha (2024-02-20)
- Linux build target: Remove accidental dependency on `glibc` by recompiling the binary using the Go-native `netgo` library instead. Checking the binary using `ldd` now reports "not a dynamic executable" as it should do. There are now no dynamic dependencies.
- Create `build.sh` script in `agent/core` to enforce this.

### v5.1.8-alpha (2024-02-20)
- Numerous bug fixes; refactoring to SystemMonitor and FeedbackResponder services.
- TCP Mode is now available again and is the default feedback mode.
- Known Issue: The JSON config needs decoupling from the object types in the code as it has ended up duplicating fields so that it ends up in a usable format.
- First Agent release ready for initial testing.
- The Configuration Tool is still in progress and will be released for alpha testing as it is available.

### v5.1.6-alpha (2024-02-16)
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
