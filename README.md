# extractor-tendermint

Blockchain data extraction utilities for Tendermint

## Usage

Initialize globally:

```go
func main() {
  extractor.SetWriterFromConfig(&extractor.Config{
    OutputFile: "/path/to/output.log",
    // Other config options are listed below
  })

  // Customize the logs prefix (DMLOG is a default)
  extractor.SetPrefix("DMLOG")

  // Initialize height frame (this is needed when using bundled output)
  extractor.SetHeight(12345)

  // Write data
  extractor.WriteLine(extractor.MsgBlock, "...block data...")
}
```

## Tendermint Instrumentation

TODO: Add this section

## Extractor Configuration

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

# We can also bundle block data per file
bundle = true
bundle_size = 1000 # each file will include data for 1k blocks/heights
# output file will be "output_file.log.0000000000" and so on

# Height range filtering
# start_height = 100
# end_height = 200
```
