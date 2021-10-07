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
# Could be one of the:
# - "stdout", "STDOUT"
# - "stderr", "STDERR"
# - "path.log" for relative to ~/.gaia directory
" - "/path/to/log.log" - for absolute path
output_file = "extractor.log"

# Height range filtering
# start_height = 100
# end_height = 200
```
