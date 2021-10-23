# extractor-tendermint

Blockchain data extraction service for Tendermint

## Tendermint Instrumentation

To enable Tendermint node instrumentation, we have to patch the node component first:

```diff
diff --git a/node/node.go b/node/node.go
index 45b92e060..d5ff5f31e 100644
--- a/node/node.go
+++ b/node/node.go
@@ -11,6 +11,7 @@ import (
 	"strings"
 	"time"
 
+	"github.com/figment-networks/extractor-tendermint"
 	"github.com/prometheus/client_golang/prometheus"
 	"github.com/prometheus/client_golang/prometheus/promhttp"
 	"github.com/rs/cors"
@@ -221,6 +222,7 @@ type Node struct {
 	txIndexer         txindex.TxIndexer
 	blockIndexer      indexer.BlockIndexer
 	indexerService    *txindex.IndexerService
+	extractorService  *extractor.ExtractorService
 	prometheusSrv     *http.Server
 }
 
@@ -307,6 +309,26 @@ func createAndStartIndexerService(
 	return indexerService, txIndexer, blockIndexer, nil
 }
 
+func createAndStartExtractorService(
+	config *cfg.Config,
+	eventBus *types.EventBus,
+	logger log.Logger,
+) (*extractor.ExtractorService, error) {
+	if !config.Extractor.Enabled {
+		logger.Info("extractor module is disabled")
+		return nil, nil
+	}
+
+	extractorService := extractor.NewExtractorService(eventBus, config.Extractor)
+	extractorService.SetLogger(logger.With("module", "extractor"))
+
+	if err := extractorService.Start(); err != nil {
+		return nil, err
+	}
+
+	return extractorService, nil
+}
+
 func doHandshake(
 	stateStore sm.Store,
 	state sm.State,
@@ -701,6 +723,11 @@ func NewNode(config *cfg.Config,
 		return nil, err
 	}
 
+	extractorService, err := createAndStartExtractorService(config, eventBus, logger)
+	if err != nil {
+		return nil, err
+	}
+
 	// If an address is provided, listen on the socket for a connection from an
 	// external signing process.
 	if config.PrivValidatorListenAddr != "" {
@@ -877,6 +904,7 @@ func NewNode(config *cfg.Config,
 		proxyApp:         proxyApp,
 		txIndexer:        txIndexer,
 		indexerService:   indexerService,
+		extractorService: extractorService,
 		blockIndexer:     blockIndexer,
 		eventBus:         eventBus,
 	}
@@ -972,6 +1000,7 @@ func (n *Node) OnStop() {
 	if err := n.eventBus.Stop(); err != nil {
 		n.Logger.Error("Error closing eventBus", "err", err)
 	}
+
 	if err := n.indexerService.Stop(); err != nil {
 		n.Logger.Error("Error closing indexerService", "err", err)
 	}
@@ -1012,6 +1041,12 @@ func (n *Node) OnStop() {
 			n.Logger.Error("Prometheus HTTP server Shutdown", "err", err)
 		}
 	}
+
+	if n.extractorService != nil {
+		if err := n.extractorService.Stop(); err != nil {
+			n.Logger.Error("Error closing extractorService", "err", err)
+		}
+	}
 }
```

Next, make sure to enable the extractor via configration. See details below.

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

# Height range filtering
# start_height = 100
# end_height = 200
```
