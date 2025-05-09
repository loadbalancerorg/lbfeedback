# Loadbalancer.org Feedback Agent: The Next Generation

## Project overview

This is the new Loadbalancer.org Feedback Agent Version 5 -- designed to boldly go where no HAProxy Feedback Agent has gone before. 

The Feedback Agent is cross-platform and concurrent, written in Go, with user-configurable multiple feedback services, non-blocking feedback responses and a statistics model that normalises reported availability to reduce unwanted fluctuations in Real Server weights. The feedback services hosted by the agent are fully configurable via a command-line interface (CLI) client, with the JSON configuration file being managed automatically.

The Agent Service provides a comprehensive JSON Application Programming Interface (API) which can be used for automating configuration tasks and reporting availability states and changing the feedback reported based on an application health check. The API is used as the mechanism by which the CLI Client controls and configures the Agent Service. Both the Agent Service and CLI Client are integrated into a single, portable binary.

The Feedback Agent service creates its own log and configuration directories, as well as a default configuration file, where these are missing. The agent will build and run "out of the box" on most POSIX platforms (currently Linux, NetBSD and macOS) and the Linux binary is intended to be as dependency-free as possible.

There is a Windows service wrapper currently in development with an accompanying Windows System Tray configuration tool.

For support, bug reports and feature requests, please contact Loadbalancer.org at <support@loadbalancer.org>.


## Installing the Feedback Agent

### Linux x86_64

#### Prerequisites
- The `lbfeedback` binary may be placed at any convenient location on the system. It is entirely self-contained and has no supporting files. This single binary acts as both the Feedback Agent system service and as the command-line interface (CLI) client which is used to control the agent.
- The Feedback Agent service must be launched from a user account that has permissions to write to the `/opt/lbfeedback` and `/var/log/lbfeedback` directories to write its JSON configuration file and logs. This may either be root (which will have the necessary write access by default) or a custom user account for which directory permissions have been granted.
- The binary is designed to be dependency-free irrespective of the platform build target; there is no requirement for `glibc` on the local system.

#### Installation steps
1. Download the most recent version of the Feedback Agent from the `binaries/` directory of this repository for the target platform. **You should not need to build the binary for yourself, as it is designed to be portable and dependency free.**
2. Decompress the Feedback Agent binary to your chosen installation directory (e.g. `/usr/bin`). Alternatively, it may be run directly from a local directory (you will need to prefix the commands below with `./` if so).
3. If upgrading from a previous version or testing a new release, please ensure any old JSON configuration files from previous Feedback Agent releases are removed from the configuration directory:<br/>
`rm -Rvf /opt/lbfeedback`
4. Launch the Feedback Agent service from a suitable startup script, or interactively for a single usage, using the following command:
`lbfeedback run-agent` (see below).

#### Running the Feedback Agent

- **Agent Service:** The binary has two "personalities"; if run with the command `lbfeedback run-agent` this will start the agent itself. This can be used either for testing the agent interactively or as the appropriate shell command to place in a startup script (e.g. an init or Upstart service, or a cron job). Note that all actions are sent to the Agent via its API to be performed and all configuration changes are automatically saved by the background Agent instance to its JSON configuration file. If the current user does not have read and write permissions for the configuration and log directories (see above) this may be launched with `sudo` if required.
- **CLI Client:** When run with any other command this launches the binary into the CLI client personality which allows it to send API commands to the running Agent. The Agent instance itself running in the background is responsible for updating the JSON configuration file and the CLI mode of the binary merely acts as an API client. The API key is fetched from the configuration file located at `/opt/lbfeedback/agent-config.json` to give the CLI personality of the binary the necessary credentials to access the agent API. The CLI Client mode does not require write access to any directories, but does require read access to the JSON configuration path above.

## Exploring the Feedback Agent's features

The steps below provide a brief tour of the basic features of the Feedback Agent, and are a useful guide to testing a new release.

### Linux x86_64

- Open a new console window and launch the Agent background service in the foreground to view the console events in real time, which are also sent to the log file:<br/>
`sudo lbfeedback run-agent`
- Verify that you are using the correct version of the Feedback Agent binary. This is printed in the masthead shown on application launch as well as the log message printed on startup.
- Observe from the console that the agent initialises with default parameters consisting of the following and writes a new configuration file:
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
- Instruct the Agent to send commands from all Responders to HAProxy to bring its RIPs into maintenance mode. By default, this is sent continuously unless overridden and the command sent to HAProxy is simply `maint`. Verify that the "maint" command continues to be sent continuously past the default command timeout:<br/>
`lbfeedback force halt`<br/>
`telnet 127.0.0.1 3333`<br/>
- Repeat also for the `drain` behaviour:
`lbfeedback force drain`<br/>
`telnet 127.0.0.1 3333`<br/>
- As above, send commands to HAProxy to force a Real Server online, and observe that by default this is only sent for 10 seconds rather than continuously:<br/>
`lbfeedback force online`<br/>
`telnet 127.0.0.1 3333`
- Where multiple Feedback Responders are configured in an Agent, to specify a single Responder only, use the `-name <responder>` parameter, where `<responder>` is the name of Feedback Responder for which the state should be forced. Test this functionality as follows:<br/>
`lbfeedback force halt -name default`<br/>
`lbfeedback force drain -name default`<br/>
`lbfeedback force online -name default`<br/>
- Next, experiment with the availability threshold. Set a minimum availability threshold below what is currently reported by the Responder above and observe the automatic commands that are now sent. An example command is as follows:<br/>
`lbfeedback set threshold -name default -threshold-max 20 -threshold-mode any`</br>
Use `stress` or a similar tool to increase CPU utilisation and observe that by default, `drain` is sent when the threshold has been reached, and `up ready` when the load is removed. If it is desired, the Agent can be configured to log any changes in command state, including from thresholds (along with the current values) using the following command. However, this should not be used in production as it may otherwise create extremely large log files:</br>
`lbfeedback edit responder -name default -log-state-changes true`</br>
Note that both the log entry and the command will be triggered on the first feedback request received by the Agent following the state change, as HAProxy commands are timed to start from the first received request.</br>

## Release Notes, Known Issues and To Do

### v5.4.0 (2025-05-09)
- v5.4.0 introduces many improvements to stability, performance and functionality throughout the Feedback Agent and the CLI and is a recommended upgrade for all users.</br>**Important Note:** The JSON configuration file format used by older versions of the Feedback Agent is not compatible with v5.4.0 due to schema changes. Before upgrading to this version, please delete any existing JSON configuration files located within `/opt/lbfeedback` on your local system.
- TLS encryption is now used for the API transport to provide enhanced security of the agent and prevent false positive scanner results from security agent software on Real Servers. HTTPS is now mandatory for access to the API endpoint.
- The CLI has been substantially improved with more useful error output and better handling of command line arguments. Required versus optional parameters are now handled correctly.
- Smart Shaping using the Z-score algorithm has now been disabled by default for all Responders. It can be re-enabled using the `-smart-shape true` parameter (via either the `add responder` or `edit responder` commands) if desired. Additionally, Smart Shaping has been refined so that the returned scores will now never exceed the minimum and maximum values calculated from the metrics for a particular Responder.
- HAProxy command threshold behaviours have been greatly enhanced based on user feedback. The specified threshold value is now the load score reported by the metric and not the returned weight; e.g. a 20% threshold means that the Responder will be brought offline if utilisation goes above 20%. A new `-threshold-mode` parameter allows options of `none` (default), `any` (any single metric or the overall score), as well as further options for setting individual thresholds per metric or for overall scores only. For further information, please refer to the help text generated by the Agent CLI.
- Multiple minor issues from user bug reports.
- The build process now uses a standard GNU makefile in place of the custom shell script that was formerly used under Linux. The agent can now be built simply by entering the root directory of the source tree and typing `make`.


### v5.3.5-beta (2024-11-11)
- Resolved an issue where the Feedback Responder is not correctly initialised if Threshold Mode is disabled. Many thanks to Neil Stone (Loadbalancer.org) for the bug report.

### v5.3.4-beta (2024-11-07)
- Resolved an issue where enabling and disabling Threshold Mode failed via the CLI due to a parameter handling problem.
- Implemented a default behaviour of enabling Threshold Mode if a valid threshold is set with `-threshold-min`, unless it is accompanied by a `-threshold-enabled false` parameter that overrides it.
- Improve `set` CLI/API commands to remove redundancy and eliminate the `cmd-` prefix, combining both command options and setting them based on whether they are specified; they can now be used as in the following example:
  - `lbfeedback set commands -name default -command-interval 30`
  - `lbfeedback set commands -name default -command-list default`
  - `lbfeedback set commands -name default -command-interval 30 -command-list default`
- Updated documentation and help text accordingly.
- Updated the `build_linux.sh` script to automatically bundle the LICENSE and README.md files in the .tar.gz archive for distribution.
- Improved internal error handling and detection.

### v5.3.3-beta (2024-11-06)
- Amended the `-name` parameter to now be optional for the `force` state  change commands. If this parameter is omitted, all Responders for which HAProxy commands are not disabled will send the specified state.
- Improved parameter validation behaviour if invalid parameters are specified in a CLI command.
- Updated help text and MVP testing instructions in README.md.

### v5.3.2-beta (2024-11-05)
- Improved state control and command timeout behaviours.

### v5.3.0/v5.3.1-beta (2024-11-05)
- Changed default behaviours to exactly match the those of the previous Windows Feedback Agent, including a threshold of 0% availability with a command interval of 10 seconds and HAProxy commands enabled. The CLI can be used to change these values if required.
- Simplified the CLI command tree to avoid the unnecessary "action" type.
- Implementation of the "netconn" and "disk-usage" System Monitor options.
- Self-documentation is now available via the "help" command of the integrated binary.

### v5.2.1-beta (2024-10-21)
- Introduction of threshold and HAProxy command support in the Agent Service mode.

### v5.1.9-alpha (2024-03-06)
- Significant improvements to the System Metric and System Monitor services.
- Enhancement of the JSON configuration syntax.
- Improvements to stability and resource handling.

### v5.1.8r2-alpha (2024-02-20)
- Linux build target: Removed dependency on `glibc` by recompiling the binary using the Go-native `netgo` library instead. Checking the binary using `ldd` now reports "not a dynamic executable" as it should do. There are now no dynamic dependencies.
- Created the `build.sh` script in `agent/core` to enforce the above build method.

### v5.1.8-alpha (2024-02-20)
- Numerous bug fixes and enhancements to the System Monitor and Feedback Responder services.

- Known Issue: The JSON config needs decoupling from the object types in the code as it has ended up duplicating fields so that it ends up in a usable format.
- First Feedback Agent v5 release ready for initial testing.
