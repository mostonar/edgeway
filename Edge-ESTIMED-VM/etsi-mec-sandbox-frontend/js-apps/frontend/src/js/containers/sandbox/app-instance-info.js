/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { connect } from 'react-redux';
import React, { Component } from 'react';
import { Typography } from '@rmwc/typography';
import { Switch } from '@rmwc/switch';
import { Grid, GridCell } from '@rmwc/grid';
import { updateObject } from '../../util/object-util';

import {
  EDGE_APP_ENABLE_COUNT_MAX,
  EDGE_APP_DISABLE_COUNT_MAX
} from '../../app-constants';

import {
  uiSandboxChangeEdgeAppList
} from '../../state/ui';

class AppInstanceInfo extends Component {
  constructor(props) {
    super(props);
  }

  changeMecServiceState(checked) {
    let edgeApp = updateObject(this.props.edgeApp, {});

    if (checked) {
      // Send event to enable service
      var enableEvent = {
        name: 'name',
        type: 'SCENARIO-UPDATE',
        eventScenarioUpdate: {
          action: 'ADD',
          node: {
            type: 'EDGE-APP',
            parent: edgeApp.mepName,
            nodeDataUnion: {
              process: {
                id: edgeApp.id,
                name: edgeApp.name,
                type: 'EDGE-APP',
                image: edgeApp.img,
                environment: edgeApp.env,
                netChar: {}
              }
            }
          }
        }
      };
      this.props.eventsApi.sendEvent('SCENARIO-UPDATE', enableEvent);

      // Enable edge app & start timer to temporarily ignore backend
      edgeApp.enabled = true;
      edgeApp.enableInProgressCount = EDGE_APP_ENABLE_COUNT_MAX;

    } else {
      // Send event to disable service
      var disableEvent = {
        name: 'name',
        type: 'SCENARIO-UPDATE',
        eventScenarioUpdate: {
          action: 'REMOVE',
          gracePeriod: 10,
          node: {
            type: 'EDGE-APP',
            parent: edgeApp.mepName,
            nodeDataUnion: {
              process: {
                id: edgeApp.id,
                name: edgeApp.name
              }
            }
          }
        }
      };
      this.props.eventsApi.sendEvent('SCENARIO-UPDATE', disableEvent);

      // Disable edge app & start timer to temporarily ignore backend 
      edgeApp.enabled = false;
      edgeApp.disableInProgressCount = EDGE_APP_DISABLE_COUNT_MAX;
    }

    // Update edge app state
    this.updateEdgeApp(edgeApp);
  }

  updateEdgeApp(edgeApp) {
    if (!this.props.edgeApps) {
      return;
    }
    let edgeApps = [];

    // Loop through edge apps
    for (var i = 0; i < this.props.edgeApps.length; i++) {
      // Add new edge app if match found; otherwise add old edge app
      if (this.props.edgeApps[i].id === edgeApp.id) {
        edgeApps.push(edgeApp);
      } else {
        edgeApps.push(updateObject(this.props.edgeApps[i], {}));
      }
    }

    // Update edge app list
    this.props.changeEdgeAppList(edgeApps);
  }

  renderStatus(edgeApp) {
    // Enable in progress
    if (edgeApp.enableInProgressCount !== -1) {
      return (
        <>
          <div style={{ marginTop: 15 }}>
            <Typography theme="primary" use="body1">MEC Service startup in progress...<br/>This operation takes a few seconds.</Typography>
          </div>
        </>
      );
    }
    // Disable in progress
    if (edgeApp.disableInProgressCount !== -1) {
      return (
        <>
          <div style={{ marginTop: 15 }}>
            <Typography theme="primary" use="body1">MEC Service shutdown in progress...<br/>This operation takes a few seconds.</Typography>
          </div>
        </>
      );
    }
    // Service disabled
    if (!edgeApp.enabled) {
      return (
        <>
          <div style={{ marginTop: 15 }}>
            <Typography theme="primary" use="body1">This MEC Service is currently disabled.</Typography>
          </div>
        </>
      );
    }
    return null;
  }

  render() {
    if (this.props.disabled || !this.props.edgeApp) {
      return null;
    }

    let edgeApp = this.props.edgeApp;
    let isMec011 = edgeApp.pseudoName.includes('011');
    let disabled = (isMec011 || edgeApp.enableInProgressCount !== -1 || edgeApp.disableInProgressCount !== -1) ? true : false;
    let mepInfoStr = (isMec011) ? edgeApp.mepName + ' platform API' : edgeApp.instance + ' running on ' + edgeApp.mepName;
    let appInfoStr = 'appInstanceId: ' + ((edgeApp.appId) ? edgeApp.appId : 'unavailable');
    let svcInfoStr = 'serviceId: ' + ((edgeApp.svcId) ? edgeApp.svcId : 'unavailable');

    return (
      <>
        <Grid style={{ marginTop: 0}}>
          <GridCell title="Enable or disable the selected MEC API" span={12}>
            <p style={{ marginTop: 5, marginBottom: 10 }}>
              <Typography theme="primary" use="headline6">MEC Service details</Typography>
            </p>
            <Typography className="text-color-main" use="body1">
              {mepInfoStr}
              {!isMec011 && <br/>}
              {!isMec011 && appInfoStr}
              {!isMec011 && <br/>}
              {!isMec011 && svcInfoStr}
            </Typography>
          </GridCell>
        </Grid>

        {!isMec011 &&
          <Grid style={{ marginTop: 10 }}>
            <GridCell span={3}>
              <Typography className={disabled && 'text-color-main'} use="body1">Enable</Typography>
            </GridCell>
            <GridCell span={9} align={'middle'} >
              <div style={{ marginLeft: 5}}>
                <Switch
                  title="Service enablement"
                  disabled={disabled}
                  checked={edgeApp.enabled}
                  onChange={(evt) => {
                    this.changeMecServiceState(evt.target.checked);
                  }}
                />
              </div>
            </GridCell>
          </Grid>
        }

        {this.renderStatus(edgeApp)}
      </>
    );
  }
}

const mapStateToProps = state => {
  return {
    edgeApps: state.ui.edgeApps
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeEdgeAppList: list => dispatch(uiSandboxChangeEdgeAppList(list))
  };
};

const ConnectedAppInstanceInfo = connect(
  mapStateToProps,
  mapDispatchToProps
)(AppInstanceInfo);

export default ConnectedAppInstanceInfo;
