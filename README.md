# ptail - persistent tailer

Ad-hoc copy/paste ball of mud for tailing files with some extra features:  
* tail multiple files  
* glob (or multiple globs) can be used to specify files (search interval configurable)
* output source filename with every line
* state can be persisted to survive restarts (persist interval configurable)
* can log what it's doing 

**use it at your own risk!**

help:  
```text
Usage of ptail:
  -file value
        file (or glob) to tail (can be used multiple times)
  -glob int
        interval in seconds for re-running glob search (default is 0 - disabled; only initially found files will be monitored). Will be auto-set to 1 if globbing detected.
  -log string
        enable logging. "-" for stdout, filename otherwise
  -persist int
        interval in milliseconds for persisting state (default is 0 - disabled)
  -sourcefile
        prefix source filename to every line (separated with tab). Will be auto-enabled if globbing or multiple files detected.
  -statefile string
        statefile to be used for persistence (default "state.json")
  -wait
        wait for files to appear, don't exit program if monitored (and actually existing) filecount is 0 (default true)
```
