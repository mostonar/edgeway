![AdvantEDGE-logo](https://github.com/InterDigitalInc/AdvantEDGE/blob/gh-pages/assets/images/AdvantEDGE-logo_Blue-01.png)

------

**_How to build the AdvantEDGE platform_**

Introducing MEC-016 support required to build demo4 example before to build the AdvantEDGE platform.

The bash script 'build_all.sh' provides the correct procedure to build the AdvantEDGE platform.

Usage:
```bash
$ cd  ~/AdvantEdge
$ ./scripts/build_all.sh
```

**_How to launch the AdvantEDGE platform_**

The bash script 'K99advantedge' is a start/stop script to manage the AdvantEDGE platform.

Usage:
```bash
$ # Get help
$ cd  ~/AdvantEdge
$ ./scripts/K99advantedge help
$ # Start the AdvantEDGE platform
$ cd  ~/AdvantEdge
$ ./scripts/K99advantedge start
$ # Stop the AdvantEDGE platform
$ cd  ~/AdvantEdge
$ ./scripts/K99advantedge stop
$ # Get current status
$ cd  ~/AdvantEdge
$ ./scripts/K99advantedge status
$ # Restart the AdvantEDGE platform
$ cd  ~/AdvantEdge
$ ./scripts/K99advantedge restart
```
