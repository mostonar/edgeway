/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { connect } from 'react-redux';
import React, { Component } from 'react';
import { Button } from '@rmwc/button';
import { Switch } from '@rmwc/switch';
import { Select } from '@rmwc/select';
import { Grid, GridCell } from '@rmwc/grid';
import { Typography } from '@rmwc/typography';
import AppInstanceTable from './app-instance-table';

import log from '../../util/logger';

import {
  deepCopy
} from '../../util/object-util';

import {
  PAGE_SANDBOX,
  DEFAULT_NO_NETWORK_FILE_SELECTED,
  NO_SCENARIO_NAME,
  ALERT_DEGRADED_NETWORK,
  DIALOG_CONFIRM_DELETE_APP,
  DIALOG_CREATE_NEW_APP,
  MAX_NB_USER_APPS,
  EDGE_APP_ENABLE_COUNT_MAX
} from '../../app-constants';

import {
  uiChangeCurrentDialog,
  uiSandboxChangeNetworkFileSelected,
  uiSandboxChangeNetworkInfo,
  uiSandboxChangePauseButton,
  uiSandboxChangeNbStationaryUe,
  uiSandboxChangeNbLowVelocityUe,
  uiSandboxChangeNbHighVelocityUe,
  uiSandboxChangeStationaryUeList,
  uiSandboxChangeLowVelocityUeList,
  uiSandboxChangeHighVelocityUeList,
  uiSandboxChangeMepList,
  uiSandboxChangeEdgeAppList,
  uiSandboxChangeMecApiSelected,
  uiSandboxChangeApiDetailedData,
  uiSandboxChangeUpdateUeInProgressCount,
  uiSandboxChangeUpdateAutomationInProgressCount,
  uiSandboxChangeUpdateAppInstancesInProgressCount,
  uiSandboxChangeTerminationInProgressCount,
  uiSandboxChangeActivationInProgressCount,
  uiSandboxChangeActivationInProgressScenarioName
} from '../../state/ui';

import {
  sboxChangeApiTable,
  sboxChangeAppInstanceTable
} from '../../state/sbox';

import {
  parseUes,
  parseMeps,
  parseEdgeApps,
  getNetworkInfo
} from '../../util/scenario-utils';

const HIGH_VELOCITY_UE = 'hv';
const LOW_VELOCITY_UE = 'lv';
const STATIONARY_UE = 'zv';

const ACTION_ADD = 'add';
const ACTION_REMOVE = 'remove';

class ConfigPane extends Component {
  constructor(props) {
    super(props);
  }

  componentDidMount() {
    log.info('componentDidMount: Configuration Pane mounted');
    if (this.props.scenario !== null && this.props.scenario.name !== NO_SCENARIO_NAME) {
      this.getScenario(this.props.scenario.name, false);
    } else {
      this.resetConfigPane();
      this.resetApiPane();
    }
  }

  componentDidUpdate(prevProps) {
    // Get configured scenario if activated by another browser
    if (this.props.activationInProgressCount === -1 && this.props.terminationInProgressCount === -1) {
      if (this.props.scenario !== null && this.props.scenario.name !== NO_SCENARIO_NAME) {
        if (this.props.networkFileSelected !== this.props.scenario.name) {
          this.getScenario(this.props.scenario.name, false);
        }
      }
    }

    // On termination completion, check if a scenario requires activation
    if (this.props.terminationInProgressCount === -1 &&
      this.props.terminationInProgressCount !== prevProps.terminationInProgressCount &&
      this.props.networkFileSelected !== DEFAULT_NO_NETWORK_FILE_SELECTED) {
      this.props.activateApi.activateScenario(this.props.networkFileSelected, null, (error, data, response) => {
        this.activateScenarioCb(error, data, response);
      });
      this.getScenario(this.props.networkFileSelected, true);
    }

    // Reset config pane if scenario was terminated by another source
    if (this.props.activationInProgressCount === -1) {
      if (this.props.scenario === null && prevProps.scenario !== null) {
        this.resetConfigPane();
        this.resetApiPane();
      }
    }
  }

  decrement(value) {
    return (value === 0) ? value : --value;
  }

  increment(type, value) {
    var maxUe = 0;

    switch(type) {
    case HIGH_VELOCITY_UE:
      maxUe = this.props.highVelocityUeList.length;
      break;
    case LOW_VELOCITY_UE:
      maxUe = this.props.lowVelocityUeList.length;
      break;
    case STATIONARY_UE:
      maxUe = this.props.stationaryUeList.length;
      break;
    default:
      break;
    }
    return (value === maxUe) ? value : ++value;
  }

  orderUeByName(a, b) {
    let comparison = 0;
    if (a.name > b.name) {
      comparison = 1;
    } else if (a.name < b.name) {
      comparison = -1;
    }
    return comparison;
  }

  resetConfigPane() {
    this.props.changeNetworkFileSelected(DEFAULT_NO_NETWORK_FILE_SELECTED);
    this.props.changeNetworkInfo('');
    this.props.changePauseButton(false);
    this.props.changeNbHighVelocityUe(0);
    this.props.changeNbLowVelocityUe(0);
    this.props.changeNbStationaryUe(0);
    this.props.changeHighVelocityUeList([]);
    this.props.changeLowVelocityUeList([]);
    this.props.changeStationaryUeList([]);
    this.props.changeAppInstanceTable([]);
    this.props.changeUpdateAppInstancesInProgressCount(2);
    this.updateAutomation(false);
  }

  resetApiPane() {
    this.props.changeMecApiSelected('');
    this.props.changeApiTable(null);
    this.props.changeApiDetailedData(null);
    this.props.changeMepList([]);
    this.props.changeEdgeAppList([]);
  }

  /**
   * Callback function to receive the result of the activateScenario operation.
   * @callback module:api/ScenarioExecutionApi~activateScenarioCallback
   * @param {String} error Error message, if any.
   */
  activateScenarioCb(error/*, data*/) {
    if (error) {
      return;
    }
    // Enable movement by default
    this.updateAutomation(false);
  }

  terminateScenarioCb(error) {
    if (error) {
      return;
    }
  }

  setScenario(name) {
    log.info(`setScenario: Setting new scenario network: ${name}`);
    // Reset config & api panes
    this.resetConfigPane();
    this.resetApiPane();

    // Set new network
    this.props.changeActivationInProgressCount(2);
    this.props.changeActivationInProgressScenarioName(name);
    this.props.changeNetworkFileSelected(name);

    if (name !== DEFAULT_NO_NETWORK_FILE_SELECTED) {
      if (this.props.scenario) {
        this.props.changeActivationInProgressCount(10);
        // Terminate running scenario before acivating new scenario
        // NOTE: new scenario will be activated when old one has been fully removed
        this.props.changeTerminationInProgressCount(30);
        this.props.activateApi.terminateScenario((error, data, response) => {
          this.terminateScenarioCb(error, data, response);
        });
      } else {
        // Activate scenario immediately if no scenario running
        this.props.activateApi.activateScenario(name, null, (error, data, response) => {
          this.activateScenarioCb(error, data, response);
        });
        this.getScenario(name, true);
      }
    } else {
      // Terminate scenario
      this.props.changeTerminationInProgressCount(30);
      this.props.activateApi.terminateScenario((error, data, response) => {
        this.terminateScenarioCb(error, data, response);
      });
    }
  }

  /**
   * Callback function to receive the result of the getScenario operation.
   * @callback module:api/ScenarioExecutionApi~getScenarioCallback
   * @param {String} error Error message, if any.
   */
  getScenarioCb(error, data/*, response*/) {
    if (error) {
      return;
    }

    // Get Network info from scenario
    var networkInfo = getNetworkInfo(data);
    this.props.changeNetworkInfo(networkInfo);

    if (this.props.activationInProgressCount === -1) {
      this.props.changeNetworkFileSelected(data.name);
    }

    // Get MEPs from scenario
    var meps = parseMeps(data);
    this.props.changeMepList(meps);

    // Get Edge Apps from scenario
    var edgeApps = parseEdgeApps(data);
    this.updateEdgeApps(edgeApps);

    // Get UEs info from scenario
    var ues = parseUes(data);

    var initHv, initLv, initZv;

    if (this.ueResetRequired) {
      initHv = ues.initHv;
      initLv = ues.initLv;
      initZv = ues.initZv;
    } else {
      initHv = this.props.nbHighVelocityUe;
      initLv = this.props.nbLowVelocityUe;
      initZv = this.props.nbStationaryUe;
    }
    var hvList = [], lvList = [], zvList = [];
    hvList = deepCopy(ues.hvList);
    lvList = deepCopy(ues.lvList);
    zvList = deepCopy(ues.zvList);

    // Sort UEs alphabetically
    hvList.sort(this.orderUeByName);
    lvList.sort(this.orderUeByName);
    zvList.sort(this.orderUeByName);

    this.props.changeHighVelocityUeList(hvList);
    this.props.changeLowVelocityUeList(lvList);
    this.props.changeStationaryUeList(zvList);

    if (this.ueResetRequired) {
    // All UEs added by default at scenario startup
    // Remove all UEs up to desired count
      var i, hv = hvList.length, lv = lvList.length, zv = zvList.length;
      for (i = hv; i > initHv; i--) {
        this.removeUe(hvList[i-1]);
        hv--;
      }
      for (i = lv; i > initLv; i--) {
        this.removeUe(lvList[i-1]);
        lv--;
      }
      for (i = zv; i > initZv; i--) {
        this.removeUe(zvList[i-1]);
        zv--;
      }

      this.props.changeNbHighVelocityUe(hv);
      this.props.changeNbLowVelocityUe(lv);
      this.props.changeNbStationaryUe(zv);
    }
  }

  getScenario(name, ueResetRequired) {
    if (name !== DEFAULT_NO_NETWORK_FILE_SELECTED && name !== '') {
      this.ueResetRequired = ueResetRequired;
      this.props.scenarioApi.getScenario(name, (error, data, response) => {
        this.getScenarioCb(error, data, response);
      });
    }
  }

  getEdgeApp(id, edgeApps) {
    if (edgeApps) {
      for (var i = 0; i < edgeApps.length; i++) {
        let edgeApp = edgeApps[i];
        if (edgeApp.id === id) {
          return edgeApp;
        }
      }
    }
    return null;
  }

  updateEdgeApps(edgeApps) {
    // Set list immediately if new or empty
    if (!edgeApps || !this.props.edgeApps) {
      this.props.changeEdgeAppList(edgeApps);
      return;
    }

    // Loop through edge apps
    for (var i = 0; i < edgeApps.length; i++) {
      let edgeApp = edgeApps[i];

      // Ignore mec011
      if (edgeApp.pseudoName.includes('011')) {
        continue;
      }

      // If Edge App already exists, use existing state
      let edgeAppInstance = this.getEdgeApp(edgeApp.id, this.props.edgeApps);
      if (edgeAppInstance !== null) {
        edgeApp.enabled = edgeAppInstance.enabled;
        edgeApp.svcId = edgeAppInstance.svcId;
        edgeApp.enableInProgressCount = edgeAppInstance.enableInProgressCount;
        edgeApp.disableInProgressCount = edgeAppInstance.disableInProgressCount;
      } else {
        // Set enable in progress for new edge app
        edgeApp.enableInProgressCount = EDGE_APP_ENABLE_COUNT_MAX;
      }
    }

    // Update Edge app list
    this.props.changeEdgeAppList(edgeApps);
  }

  updateAutomation(pauseButtonEnabled) {
    this.props.changeUpdateAutomationInProgressCount(2);
    this.props.automationApi.setAutomationStateByName('MOVEMENT', !pauseButtonEnabled);
    this.props.automationApi.setAutomationStateByName('MOBILITY', !pauseButtonEnabled);
  }

  addUe(element) {
    var sandboxEvent = {
      name: 'name',
      type: 'SCENARIO-UPDATE',
      eventScenarioUpdate: {
        action: 'ADD',
        node: {
          type: 'UE',
          parent: element.parentName,
          nodeDataUnion: {
            physicalLocation: {
              name: element.name,
              type: 'UE',
              macId: element.macId,
              connected: element.connected,
              wireless: element.wireless,
              wirelessType: element.wirelessType,
              geoData: element.geoData,
              meta: element.meta,
              netChar: {},
              processes: element.processes ? deepCopy(element.processes) : []
            }
          }
        }
      }
    };
    this.props.eventsApi.sendEvent('SCENARIO-UPDATE', sandboxEvent);
  }

  removeUe(element) {
    var sandboxEvent = {
      name: 'name',
      type: 'SCENARIO-UPDATE',
      eventScenarioUpdate: {
        action: 'REMOVE',
        node: {
          type: 'UE',
          parent: element.parentName,
          nodeDataUnion: {
            physicalLocation: {
              name: element.name,
              type: 'UE'
            }
          }
        }
      }
    };
    this.props.eventsApi.sendEvent('SCENARIO-UPDATE', sandboxEvent);
  }

  updateUe(action, type, ueIndex) {
    //not a valid lower bound index
    if (ueIndex < 0) {
      return;
    }

    var list;
    switch(type) {
    case HIGH_VELOCITY_UE:
      if (ueIndex > this.props.highVelocityUeList.length) {
        return;
      }
      list = this.props.highVelocityUeList;
      break;
    case LOW_VELOCITY_UE:
      if (ueIndex > this.props.lowVelocityUeList.length) {
        return;
      }
      list = this.props.lowVelocityUeList;
      break;
    case STATIONARY_UE:
      if (ueIndex > this.props.stationaryUeList.length) {
        return;
      }
      list = this.props.stationaryUeList;
      break;
    default:
      return;
    }

    switch(action) {
    case ACTION_ADD:
      this.props.changeUpdateUeInProgressCount(2);
      this.addUe(list[ueIndex-1]);
      break;
    case ACTION_REMOVE:
      this.props.changeUpdateUeInProgressCount(2);
      this.removeUe(list[ueIndex]);
      break;
    default:
      break;
    }
  }

  render() {
    if (this.props.page !== PAGE_SANDBOX) {
      return null;
    }

    const terminationInProgress = (this.props.terminationInProgressCount !== -1) ? true : false;
    const networkFileSelected = (this.props.networkFileSelected !== DEFAULT_NO_NETWORK_FILE_SELECTED) ? true : false;
    const configEnabled = (networkFileSelected && !terminationInProgress) ? true : false;
    const networkFileSelectionEnabled = (this.props.activationInProgressCount === -1) ? true : false;
    const maxNbStationaryUe = (this.props.stationaryUeList) ? this.props.stationaryUeList.length : 0;
    const maxNbLowVelocityUe = (this.props.lowVelocityUeList) ? this.props.lowVelocityUeList.length : 0;
    const maxNbHighVelocityUe = (this.props.highVelocityUeList) ? this.props.highVelocityUeList.length : 0;

    var nbUserApps = 0;
    if (this.props.appInstanceTable) {
      for (var i = 0; i < this.props.appInstanceTable.length; i++) {
        if (this.props.appInstanceTable[i].type === 'USER') {
          nbUserApps++;
        }
      }
    }

    return (
      <div style={{ padding: 15 }}>
        <div style={{ marginBottom: 20}} title="Configure a MEC scenario">
          <Typography theme="primary" use="headline6">Configuration</Typography>
        </div>

        <Grid>
          <GridCell span={12}>
            <Select
              title="Select the network to simulate"
              style={styles.select}
              label="Simulated Network"
              fullwidth="true"
              outlined
              disabled={!networkFileSelectionEnabled}
              options={this.props.networkFiles}
              onChange={event => {
                this.setScenario(event.target.value);
              }}
              value={this.props.networkFileSelected}
            />
          </GridCell>
        </Grid>

        {!terminationInProgress &&
          <div style={!configEnabled ? {pointerEvents: 'none', opacity: '0.3', marginLeft: 5} : { marginLeft: 5 }}>
            <Grid style={{ marginTop: 30, marginBottom: 20}}>
              <GridCell span={5}>
                <Typography use="body1">Pause</Typography>
              </GridCell>
              <GridCell span={6} align={'middle'} >
                <div style={{ marginLeft: 5}}>
                  <Switch
                    title="Pause UE movement"
                    disabled={!configEnabled}
                    checked={this.props.pauseButton}
                    onChange={() => {
                      this.updateAutomation(!this.props.pauseButton);
                      this.props.changePauseButton(!this.props.pauseButton);
                    }}
                  />
                </div>
              </GridCell>
            </Grid>

            <Grid style={styles.row}>
              <GridCell span={5}>
                <Typography use="body1">Stationary UE</Typography>
              </GridCell>
              <GridCell span={6}>
                <Button
                  title="Remove UE"
                  style={styles.smalltextGrayButton}
                  label="-"
                  raised
                  onClick={() => {
                    this.updateUe(ACTION_REMOVE, STATIONARY_UE, this.props.nbStationaryUe - 1);
                    this.props.changeNbStationaryUe(this.decrement(this.props.nbStationaryUe));
                  }}
                >-
                </Button>
                <Button
                  style={styles.smalltextWhiteButtonWide}
                  label="nbStationaryUe"
                  raised
                >
                  <Typography use="body1">{this.props.nbStationaryUe} / {maxNbStationaryUe}</Typography>
                </Button>
                <Button
                  title="Add UE"
                  style={styles.smalltextGrayButton}
                  label="-"
                  raised
                  onClick={() => {
                    this.updateUe(ACTION_ADD, STATIONARY_UE, this.props.nbStationaryUe + 1);
                    this.props.changeNbStationaryUe(this.increment(STATIONARY_UE, this.props.nbStationaryUe));
                  }}
                >+
                </Button>
              </GridCell>
            </Grid>

            <Grid style={styles.row}>
              <GridCell span={5}>
                <Typography use="body1">Low Velocity UE</Typography>
              </GridCell>
              <GridCell span={6}>
                <Button
                  title="Remove UE"
                  style={styles.smalltextGrayButton}
                  label="-"
                  raised
                  onClick={() => {
                    this.updateUe(ACTION_REMOVE, LOW_VELOCITY_UE, this.props.nbLowVelocityUe - 1);
                    this.props.changeNbLowVelocityUe(this.decrement(this.props.nbLowVelocityUe));
                  }}
                >-
                </Button>
                <Button
                  style={styles.smalltextWhiteButtonWide}
                  label="nbLowVelocityUe"
                  raised
                >
                  <Typography use="body1">{this.props.nbLowVelocityUe} / {maxNbLowVelocityUe}</Typography>
                </Button>
                <Button
                  title="Add UE"
                  style={styles.smalltextGrayButton}
                  label="-"
                  raised
                  onClick={() => {
                    this.updateUe(ACTION_ADD, LOW_VELOCITY_UE, this.props.nbLowVelocityUe + 1);
                    this.props.changeNbLowVelocityUe(this.increment(LOW_VELOCITY_UE, this.props.nbLowVelocityUe));
                  }}
                >+
                </Button>
              </GridCell>
            </Grid>

            <Grid style={styles.row}>
              <GridCell span={5}>
                <Typography use="body1">High Velocity UE</Typography>
              </GridCell>
              <GridCell span={6}>
                <Button
                  title="Remove UE"
                  style={styles.smalltextGrayButton}
                  label="-"
                  raised
                  onClick={() => {
                    this.updateUe(ACTION_REMOVE, HIGH_VELOCITY_UE, this.props.nbHighVelocityUe -1);
                    this.props.changeNbHighVelocityUe(this.decrement(this.props.nbHighVelocityUe));
                  }}
                >-
                </Button>
                <Button
                  style={styles.smalltextWhiteButtonWide}
                  label="nbHighVelocityUe"
                  raised
                >
                  <Typography use="body1">{this.props.nbHighVelocityUe} / {maxNbHighVelocityUe}</Typography>
                </Button>
                <Button
                  title="Add UE"
                  style={styles.smalltextGrayButton}
                  label="-"
                  raised
                  onClick={() => {
                    this.updateUe(ACTION_ADD, HIGH_VELOCITY_UE, this.props.nbHighVelocityUe + 1);
                    this.props.changeNbHighVelocityUe(this.increment(HIGH_VELOCITY_UE, this.props.nbHighVelocityUe));
                  }}
                >+
                </Button>
              </GridCell>
            </Grid>

            <Grid style={styles.rowAppInstance}>
              <GridCell align={'middle'} span={5}>
                <Typography use="body1">MEC App. Instance IDs</Typography>
              </GridCell>
              <GridCell span={6}>
                <Button
                  title="create a new application instance (max: 10)"
                  style={styles.button}
                  disabled={nbUserApps >= MAX_NB_USER_APPS}
                  onClick={() => {
                    this.props.changeCurrentDialog(DIALOG_CREATE_NEW_APP);
                  }}
                >NEW
                </Button>
                <Button
                  title="delete selected application instances"
                  style={styles.button}
                  disabled={this.props.appInstancesSelected.length === 0}
                  onClick={() => {
                    this.props.changeCurrentDialog(DIALOG_CONFIRM_DELETE_APP);
                  }}
                >DELETE
                </Button>
              </GridCell>
            </Grid>

            <AppInstanceTable/>
          </div>
        }

        {networkFileSelected && terminationInProgress &&
          <Grid>
            <GridCell span={12}>
              <div style={{ marginTop: 15 }}>
                <Typography theme="primary" use="body1">
                  Network activation in progress...<br/>
                  This operation takes a few seconds. <br/>
                </Typography>
              </div>
            </GridCell>
          </Grid>
        }

        {!networkFileSelected && terminationInProgress &&
          <Grid>
            <GridCell span={12}>
              <div style={{ marginTop: 15 }}>
                <Typography theme="primary" use="body1">
                  Network termination in progress...<br/>
                  This operation takes a few seconds.
                </Typography>
              </div>
            </GridCell>
          </Grid>
        }

        <div hidden={!this.props.currentAlert} style={{ width: '100%', borderTop: '2px solid #e4e4e4', marginTop: 30, marginBottom: 10}} />

        <div hidden={(this.props.currentAlert !== ALERT_DEGRADED_NETWORK)}>
          <Grid>
            <GridCell span={12}>
              <Typography use="subtitle2" style={{color: 'orangered'}}>
                <b>WARNING</b> - Degraded connectivity.<br />Controls may not function properly at this time.
              </Typography>
            </GridCell>
          </Grid>
        </div>

      </div>
    );
  }
}

const styles = {
  smalltextGrayButton: {
    color: 'black',
    fontFamily: 'Gill Sans, Gill Sans MT, Calibri, Trebuchet MS, sans-serif',
    fontSize: 16,
    height: 22,
    width: 38,
    minWidth: 38,
    backgroundColor: 'lightgray'
  },
  bigtextGrayButton: {
    color: 'black',
    fontFamily: 'Gill Sans, Gill Sans MT, Calibri, Trebuchet MS, sans-serif',
    fontSize: 12,
    height: 22,
    width: 80,
    minWidth: 80,
    backgroundColor: 'lightgray'
  },
  smalltextWhiteButtonWide: {
    color: 'black',
    fontFamily: 'Gill Sans, Gill Sans MT, Calibri, Trebuchet MS, sans-serif',
    fontSize: 16,
    height: 22,
    width: 75,
    minWidth: 75,
    backgroundColor: 'white',
    paddingRight: 0,
    paddingLeft: 0
  },
  button: {
    marginRight: 10
  },
  row: {
    fontSize: 20,
    fontFamily: 'Gill Sans, Gill Sans MT, Calibri, Trebuchet MS, sans-serif',
    marginBottom: 20
  },
  rowAppInstance: {
    marginTop: 25,
    marginBottom: 15
  },
  select: {
    width: '100%'
  }
};

const mapStateToProps = state => {
  return {
    page: state.ui.page,
    currentAlert: state.ui.currentAlert,
    networkFiles: state.ui.networkFiles,
    networkFileSelected: state.ui.networkFileSelected,
    networkInfo: state.ui.networkInfo,
    pauseButton: state.ui.pauseButton,
    nbStationaryUe: state.ui.nbStationaryUe,
    nbLowVelocityUe: state.ui.nbLowVelocityUe,
    nbHighVelocityUe: state.ui.nbHighVelocityUe,
    stationaryUeList: state.ui.stationaryUeList,
    lowVelocityUeList: state.ui.lowVelocityUeList,
    highVelocityUeList: state.ui.highVelocityUeList,
    edgeApps: state.ui.edgeApps,
    appInstanceTable: state.sbox.appInstanceTable.data,
    scenario: state.sbox.scenario,
    appInstancesSelected: state.ui.appInstancesSelected,
    terminationInProgressCount: state.ui.terminationInProgressCount,
    activationInProgressCount: state.ui.activationInProgressCount,
    activationInProgressScenarioName: state.ui.activationInProgressScenarioName
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeCurrentDialog: type => dispatch(uiChangeCurrentDialog(type)),
    changeTerminationInProgressCount: count => dispatch(uiSandboxChangeTerminationInProgressCount(count)),
    changeActivationInProgressCount: count => dispatch(uiSandboxChangeActivationInProgressCount(count)),
    changeActivationInProgressScenarioName: name => dispatch(uiSandboxChangeActivationInProgressScenarioName(name)),
    changeUpdateUeInProgressCount: count => dispatch(uiSandboxChangeUpdateUeInProgressCount(count)),
    changeUpdateAutomationInProgressCount: count => dispatch(uiSandboxChangeUpdateAutomationInProgressCount(count)),
    changeUpdateAppInstancesInProgressCount: count => dispatch(uiSandboxChangeUpdateAppInstancesInProgressCount(count)),
    changeNetworkFileSelected: name => dispatch(uiSandboxChangeNetworkFileSelected(name)),
    changeNetworkInfo: value => dispatch(uiSandboxChangeNetworkInfo(value)),
    changePauseButton: checked => dispatch(uiSandboxChangePauseButton(checked)),
    changeNbStationaryUe: value => dispatch(uiSandboxChangeNbStationaryUe(value)),
    changeNbLowVelocityUe: value => dispatch(uiSandboxChangeNbLowVelocityUe(value)),
    changeNbHighVelocityUe: value => dispatch(uiSandboxChangeNbHighVelocityUe(value)),
    changeStationaryUeList: value => dispatch(uiSandboxChangeStationaryUeList(value)),
    changeLowVelocityUeList: value => dispatch(uiSandboxChangeLowVelocityUeList(value)),
    changeHighVelocityUeList: value => dispatch(uiSandboxChangeHighVelocityUeList(value)),
    changeMepList: value => dispatch(uiSandboxChangeMepList(value)),
    changeEdgeAppList: value => dispatch(uiSandboxChangeEdgeAppList(value)),
    changeMecApiSelected: name => dispatch(uiSandboxChangeMecApiSelected(name)),
    changeApiDetailedData: row => dispatch(uiSandboxChangeApiDetailedData(row)),
    changeApiTable: value => dispatch(sboxChangeApiTable(value)),
    changeAppInstanceTable: value => dispatch(sboxChangeAppInstanceTable(value))
  };
};

const ConnectedConfigPane = connect(
  mapStateToProps,
  mapDispatchToProps
)(ConfigPane);

export default ConnectedConfigPane;
