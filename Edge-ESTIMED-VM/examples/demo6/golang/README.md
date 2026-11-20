Demo 6 is a MEC application to illustrate the usage of the MEC Sandbox command line APIs.

# How to use it

## Building the demo6 application

go version 1.18+ is required to build demo 6

There is two ways to build demo6 application:
- Using the docker_build.sh script

```sh
~$ docker pull golang
~$ cd ~/etsi-mec-sandbox/examples/demo6 
~/etsi-mec-sandbox/examples/demo6$ docker_build.sh
```

- Manually:

```sh
~$ docker pull golang
~$ cd ~/etsi-mec-sandbox/examples/demo6 
~/etsi-mec-sandbox/examples/demo6$ docker run --rm -it -v$PWD:/opt/local/etsi/demo6 golang
root@56c7b1ce74ca:/go# cd /opt/local/etsi/demo6
root@56c7b1ce74ca:/opt/local/etsi/demo6# go run ./main.go
```

## Executing the demo6 application

The demo6 application can be executed using the script run.sh:

```sh
~$ cd ~/etsi-mec-sandbox/examples/demo6 
~/etsi-mec-sandbox/examples/demo6$ docker_run.sh
```

# Menu description

Below is the menu proposed by the demo6 application:

```sh
Mandatory commands:
        l: Login, n: Get Namespace, L: Logout, s: Get scenarios list
        a <index>: Activate a scenario, A: Terminate a scenario
Optional commands:
        S <index>: Get scenario description
        m: Get MEC application services list
        i: Get application instances list, c: Create a new application instance, C: Delete a new application instance
        u: Get UEs, x <index>: Increase UE, X <index>: Decrease UE
        T: Current status:
MEC 011 App Support:
        y: Send ConfirmReady, r: Send Registration, R: Send Deregistration
MEC 011 Service Management:
        v: Create new service, V: Delete service, g: Get list of MEC services
MEC 030:
        Y: Get V2X UU unicast setting', z: V2X Msg subscription, Z <subID>: Delete V2X subscription, Q <[latutudes] [longitudes] [timestamps]: Provide PredictedQoS
                [latitudes] is a set of latitudes separated by comma, [longitudes] is a set of longitudes separated by comma, [timestamps]
                E.g. 43.729416,43.732456 7.414853,7.418417 1653295620,1653299220
MEC 040:
        0: Get Federation Systems list', 1 <systemId>: Get Federation Services list, 2 <systemId> <serviceId>: Get Federation Service, 3: Subscribe, 4 [<subId>]: Get subscription, 5 <subId>: Delete subscription
q: Quit
Enter your choice:
```

## Mandatories commands

Mandatories commands are the sequence of command to execute before to execute with the optionals commands.

### Login command

The Login command (l) authenticates and authorizes the MEC application demo6 and creates a new MEC sandbox instance.

### Logout command

The Logout command (L) terminates an existing MEC sandbox instance.

### Get scenarios list

After the login step, it is mandatory to request the list of available network scenarios in order to execute with the optional commands.

## Optional commands

### Get scenario description

This option (S) uses the index of the network scenario (starting from 0 i.e. S 0) to retrieve the description of the selected network scenario.

### Activate a network scenario

This option (a) uses the index of the network scenario (starting from 0 i.e. a 0) to activate the selected network scenario. This command requests the MEC Sandbox instance to start all the MEC services attached to the activated network scenario.

### Deactivate a network scenario

This option (d) uses the index of the network scenario (starting from 0 i.e. d 0) to deactivate the selected network scenario. This command requests the MEC Sandbox instance to terminate all the MEC services attached to the activated network scenario.

### Get MEC services list

This option (m) retrieves the list of the MEC services available with the activated network scenario.

### Get application instances list

This option (i) retrieves the list of the MEC applications available with the activated network scenario.

### Create a new application instance

This option (c) creates the a new USER MEC application. The application instance id is generated automatically and its name is "demo6".
This new application is attached to "mep1".

Note: Additional new application instances can be created to simulate several MEC application. This demo application is limited to one new USER MEC application.

### Delete a new application instance

This option (C) terminates an existing MEC application.

## MEC Services commands

Theses commands provides some example of access to the MEC services available with the activated network scenario.

### MEC Application Support (MEC 011 service)

- The option (y) sends a confirm-ready to the MEC platform to indicate a READY state.
- The option (r) registers the MEC application to the MEC platform (optional).
- The option (R) de-registers the MEC application from the MEC platform (optional).

### MEC Service Managenment (MEC 011 service)


### Get V2X UU unicast setting (MEC 030 service)

- The option (Y) retrieves the UU Unicast settings from the MEC 030 service. To use it, the V2X network scenario shall be activated (see [Activate a network scenario](#activate_a_network_scenario)).
- The option (z) creates a subscription on V2X messages.
- The option (Z) deletes a subscription.

## Application status

The option (Q) provides the demo6 application status. If a Login commands was executed, the termination process does the logout.

```
Current status: Sandbox: sbxqvgq4q4, appsInfo.Id: d75d8536-cf94-45a6-91a3-fc5b033f9630, Subscription: sub-wwJ_uzHOwqPntaia, Demo6 app not registered
```

In the example above, the MEC Platform identifier is sbxqvgq4q4, the MEC application identifier is d75d8536-cf94-45a6-91a3-fc5b033f9630 and a subscription to the applicationTerminationNotifcation was created: sub-wwJ_uzHOwqPntaia. Note that the MEC application is not registered to the MEC Platform.

## Terminate the demo6 application

The command Quit (q) terminates the demo6 application. If a Login commands was executed, the termination process does the logout, deletes all subscriptions, de-registers the MEC appliction if a register was done and terminates an existing MEC sandbox instance.
