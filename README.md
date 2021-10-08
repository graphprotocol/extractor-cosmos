# extractor-tendermint

Blockchain data extraction service for Tendermint


## Tendermint configuration

To enable extractor module, add the following to your `~/.gaia/config/config.toml` file:

```toml
#######################################################
###           Extractor Configuration Options       ###
#######################################################

[extractor]
# Enable the extractor workflow
enabled = true

# Output log file for all extracted data
#
# Could be one of the:
# - "stdout", "STDOUT" will print out logs to standard output
# - "stderr", "STDERR" will print out logs to standard error output
# - "path.log" for relative to ~/.gaia directory
# - "/path/to/log.log" - for absolute path
#
# Supported variables in the filename:
# - $date - replaced by YYYYMMDD of current time in UTC
# - $time - replaced by HHMMSS of current time in UTC
# - $ts - replaced by a YYYYMMDD-HHMMSS of current time in UTC
#
output_file = "extractor.log"
# output_file = "extractor-$date.log"
# output_file = "extractor-$ts.log"

# Height range filtering
# start_height = 100
# end_height = 200
```
