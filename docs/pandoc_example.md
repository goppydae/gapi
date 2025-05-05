% COMMAND(SECTION) Project Manual
% Author Name
% Month Year

# NAME

command - short description of what the command does

# SYNOPSIS

**command** [*options*] [*arguments*]

# DESCRIPTION

A short paragraph or two describing what the command is for,
how it fits into the system, and what its primary responsibilities are.

# COMMANDS

**start** *<agent>*
: Start a specific supervised agent.

**stop** *<agent>*
: Stop a supervised agent.

**status**
: Display current status of all known agents.

**describe** *<agent>*
: Print metadata for an agent.

# OPTIONS

**--help**
: Show help message and exit.

**--debug**
: Enable debug output.

**--json**
: Print output in JSON format.

# FILES

`/etc/myapp/`
: Default configuration directory.

`/var/log/myapp/`
: System logs.

# ENVIRONMENT

`MYAPP_CONFIG`
: Override the default config path.

# SEE ALSO

**othercmd(1)**, **agentctl(8)**

# AUTHOR

Written by Your Name.
