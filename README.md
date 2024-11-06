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

## Background information
*Please accept my sincerest apologies for the brevity of these notes, which I will update as time permits. -- NT*
- It does not matter where the binary is placed, as it will run from any directory. However, in order to write its logs and configuration files, it must be run with root permissions.
- The binary is designed to be dependency-free irrespective of the platform build target; all required libraries should be compiled in.
### Linux x86_64
- Download the most recent version of the Feedback Agent from the `binaries/` directory of this repository for the target platform. **You do not need to build the binary for yourself as it is designed to be portable and dependency free.**
- Either untar the binary to a suitable location in your PATH (e.g. `/usr/bin`) or run directly from a local directory (you will need to prefix the commands below with `./` if so).
- The binary has two "personalities"; if run with the command `lbfeedback run-agent` (as `sudo`) this will start the agent itself. This can be used either for testing the agent interactively or as the appropriate shell command to place in a startup script (e.g. an init or Upstart service, or a cron job). The user will not have to do this manually as Andy's install script (via a link from the Portal) will create the appropriate startup service. Note that all actions are sent to the Agent via its API to be performed and all configuration changes are automatically saved by the background Agent instance to its JSON configuration file.
- When run with any other command this launches the binary into the CLI client personality which allows it to send API commands to the running Agent. The Agent instance itself running in the background is responsible for updating the JSON configuration file and the CLI mode of the binary merely acts as an API client. The API key is fetched from the configuration file located at `/opt/lbfeedback/agent-config.json` to give the CLI personality of the binary the necessary credentials to access the agent API. The CLI Client personality does not require `sudo` privileges, whereas the Agent service mode does.
## Initial MVP testing instructions

### Linux x86_64

- First, please ensure any old JSON configuration files from previous Feedback Agent testing are removed from the configuration directory:<br/>
`rm -Rvf /opt/lbfeedback`
- Open a new console window and launch the Agent background service in the foreground to view the console events in real time, which are also sent to the log file:<br/>
`sudo lbfeedback run-agent`
- Verify that you are testing the correct version of the Feedback Agent binary, which at the time of writing is `5.3.4-beta`. This is printed in the masthead shown on application launch as well as the log message printed on startup.
- Verify from the console that the agent initialises with default parameters consisting of the following and writes a new configuration file:
  - A single CPU mode System Monitor named "cpu".
  - A single TCP mode Responder listening on all IPs on port 3333 named "default", with a single monitor source of the "cpu" default monitor.
  - An HTTP mode API Responder listening on 127.0.0.1 on port 3334.
- Open a separate terminal window for testing the agent behaviour.
- Use Telnet to verify that the TCP mode responder is providing feedback followed by a line break and TCP FIN (not `nc` as it seems to stay attached to stdin and doesn't indicate the TCP FIN) as follows:<br/>
`telnet 127.0.0.1 3333`
- Show the basic help documentation provided by the Agent (this gives an idea of the commands):<br/>
`lbfeedback help`<br/>
- Get the running configuration state of the agent:<br/>
`lbfeedback get config`
- Create a new RAM type System Monitor with default settings named "ram":<br/>
`lbfeedback add monitor -name ram -metric-type ram`
- Add this new monitor as a source to the existing Responder named "default":<br/>
`lbfeedback add source -name default -monitor ram`
- Change the significance of the RAM monitor within the Responder from 1.0 to 0.5 resulting in its relative significances being recalculated:<br/>
`lbfeedback edit source -name default -monitor ram -significance 0.5`<br/>
The total of all significance values should now be reported in the log as 1.50 with a resulting relative significance of 0.67 for the CPU monitor and 0.33 for the RAM monitor, as follows:<br/>
~~~
INFO[2024-11-05 12:29:47] Responder 'default' : calculating relative significances, total 1.50. 
INFO[2024-11-05 12:29:47] Responder 'default: name 'cpu', type 'cpu': 1.00 -> relative 0.67. 
INFO[2024-11-05 12:29:47] Responder 'default: name 'ram', type 'ram': 0.50 -> relative 0.33.
~~~
- Recheck the feedback to show that this change in significance has taken effect:<br/>
`telnet 127.0.0.1 3333`
- Instruct the Agent to send commands from all Responders to HAProxy to bring its RIPs into maintenance mode. By default this is sent continuously unless overridden and the command is simply `maint`. Verify that the "maint" command continues to be sent continuously past the default command timeout:<br/>
`lbfeedback force halt`<br/>
`telnet 127.0.0.1 3333`<br/>
- Repeat also for the `drain` behaviour:
`lbfeedback force drain`<br/>
`telnet 127.0.0.1 3333`<br/>
- As above, send commands to HAProxy to force a RIP online but observe this time that it is only sent for 10 seconds:<br/>
`lbfeedback force online`<br/>
`telnet 127.0.0.1 3333`
- Where multiple Feedback Responders are configured in an Agent, to specify a single Responder only, use the `-name <responder>` parameter, where `<responder>` is the name of Feedback Responder for which the state should be forced. Test this functionality as follows:<br/>
`lbfeedback force halt -name default`<br/>
`lbfeedback force drain -name default`<br/>
`lbfeedback force online -name default`<br/>
- Next, check the availability threshold functionality. Set a minimum availability threshold below what is currently reported by the Responder above and observe the automatic commands that are now sent. (Note that by default, setting a valid threshold will automatically enable thresholding for a Responder, but if desired this can be overridden by adding the `-threshold-enabled false` option.) Test the following command:
`lbfeedback set threshold -name default -threshold-min 60`</br>
Use `stress` or a similar tool to increase CPU utilisation and observe that by default, `drain` is sent when the threshold has been reached, and `up ready` when the load is removed.

## Release Notes, Known Issues and To Do

### v5.3.4-beta (2024-11-07)
- Fix an issue where enabling and disabling Threshold Mode did not work properly via the CLI due to a parameter handling bug.
- Implement a default behaviour of enabling Threshold Mode if a valid threshold is set with `-threshold-min`, unless it is accompanied by a `-threshold-enabled false` parameter that overrides it.
- Improve `set` CLI/API commands to remove redundancy and eliminate the `cmd-` prefix, combining both command options and setting them based on whether they are specified; they can now be used as in the following example:
  - `lbfeedback set commands -name default -command-interval 30`
  - `lbfeedback set commands -name default -command-list default`
  - `lbfeedback set commands -name default -command-interval 30 -command-list default`
- Update documentation and help text accordingly.
- Update the `build_linux.sh` script to automatically bundle the LICENSE and README.md files in the .tar.gz archive for distribution.
- Prevent a possible runtime panic by implementing a missing null field check in the API handler for configuring HAProxy commands.

### v5.3.3-beta (2024-11-06)
- Make the `-name` parameter optional for the `force` commands. If this parameter is omitted, all Responders for which HAProxy commands are not disabled will send the specified state.
- Clean up the parameter validation behaviour if invalid parameters are specified in a CLI command.
- Update help text and MVP testing instructions in README.md.

### v5.3.2-beta (2024-11-05)
- Change force/set behaviours and command timeout behaviours as per Malcolm.

### v5.3.0/v5.3.1-beta (2024-11-05)
- Change behaviours to exactly match the Windows Feedback Agent, including a default threshold of 0% availability with a command interval of 10 seconds and commands enabled by default, as requested by Malcolm. Personally, I think that these defaults need to be reviewed (especially the command interval and having commands enabled by default), but in any case, the CLI can be used to change this based on the customer's requirements.
- Simplify CLI command tree to avoid the unnecessary "action" type.
- Implement the "netconn" and "disk-usage" System Monitor options.
- Provide self-documentation via the "help" command.


### v5.2.1-beta (2024-10-21)
- A massive number of changes for the Linux MVP release.
- Known issues:
  - Colours from the `logrus` library are making their way through into the logfile in `/var/log/lbfeedback` which is very annoying. Strangely, this is harder to fix than you might imagine as the option to disable colours also has the effect of disabling the formatting.
  - The error output from the CLI client mode is not very user-friendly.
  - There is no self-documentation yet in the binary; that is, a `help` command is currently missing.
  - There are almost no default parameters where these don't need to be specified.
  - The CLI personality lacks prevalidation of parameters before sending to the API.
  - A more human-friendly output than the pretty-printed JSON would be nice for the CLI result.
  - Nick has not yet documented the CLI/API - there is a tonne of available options and commands (including adding, removing, editing stuff) but this needs to be written up.

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
