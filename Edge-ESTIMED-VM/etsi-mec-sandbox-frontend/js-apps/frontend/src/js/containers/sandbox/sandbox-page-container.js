/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import _ from 'lodash';
import { connect } from 'react-redux';
import React, { Component } from 'react';
import { Grid, GridCell } from '@rmwc/grid';
import { Elevation } from '@rmwc/elevation';
import MapInfo from './map-info';
import ConfigPane from './configuration-pane';
import ApiPane from './api-pane';

import DetailedDataDialog from '../../components/dialogs/detailed-data-dialog';

import {
  uiChangeCurrentDialog,
  uiSandboxChangeNetworkFilesList
} from '../../state/ui';

import {
  PAGE_SANDBOX,
  DEFAULT_NO_NETWORK_FILE_SELECTED,
  DIALOG_DETAILED_DATA
} from '../../app-constants';


class SandboxPageContainer extends Component {
  constructor(props) {
    super(props);
  }

  componentDidMount() {
    this.updateNetworkFileList();
  }

  // Update configuration page state
  setPageState(state) {
    this.props.changeState(state);
  }


  /**
   * Callback function to receive the result of the getScenarioList operation.
   * @callback module:api/ScenarioConfigurationApi~getScenarioListCallback
   * @param {String} error Error message, if any.
   * @param {module:model/ScenarioList} data The data returned by the service call.
   * @param {String} response The complete HTTP response.
   */
  getScenarioListLoadCb(error, data /*, response*/) {
    if (error !== null) {
      // TODO: consider showgina an alert
      return;
    }
    if (!data.scenarios) {
      return;
    }
    let files = _.map(data.scenarios, 'name');
    files.unshift(DEFAULT_NO_NETWORK_FILE_SELECTED);
    this.props.changeNetworkFilesList(files);

  }

  updateNetworkFileList() {
    this.props.scenarioApi.getScenarioList((error, data, response) => {
      this.getScenarioListLoadCb(error, data, response);
    });
  }

  closeDialog() {
    this.props.changeCurrentDialog(null);
  }

  renderDialogs() {
    return (
      <>
        <DetailedDataDialog
          title="Detailed info"
          open={this.props.currentDialog === DIALOG_DETAILED_DATA}
          onClose={() => {
            this.closeDialog();
          }}
          detailedData={this.props.detailedData}
        />
      </>
    );
  }

  render() {
    if (this.props.page !== PAGE_SANDBOX) {
      return null;
    }

    return (
      <div>
        {this.renderDialogs()}

        <Grid style={styles.headlineGrid}>
          <GridCell span={12}>
            <div style={{ display: 'flex', height: '100%' }}>
              <div style={{ flex: '1', marginRight: 10}}>
                <Elevation
                  className="component-style"
                  z={4}
                >
                  <div style={{ padding: 10, height: '60vh' }}>
                    <MapInfo
                      page={this.props.page}
                    />
                  </div>
                </Elevation>
              </div>
              <div
                className="component-style"
                style={{ flex: '0 0 450px' }}
              >
                <Elevation
                  // className="component-style"
                  z={4}
                  style={{ height: '100%' }}
                >
                  <ConfigPane
                    page={this.props.page}
                    scenarioApi = {this.props.scenarioApi}
                    activateApi = {this.props.activateApi}
                    eventsApi = {this.props.eventsApi}
                    automationApi = {this.props.automationApi}
                  />
                </Elevation>
              </div>
            </div>
          </GridCell>

          <GridCell span={12}>
            <Elevation
              className="component-style"
              z={4}
            >
              <ApiPane
                page={this.props.page}
                scenarioApi = {this.props.scenarioApi}
                activateApi = {this.props.activateApi}
                metricsApi = {this.props.metricsApi}
                eventsApi = {this.props.eventsApi}
                applicationsApi={this.props.applicationsApi}
              />
            </Elevation>
          </GridCell>
        </Grid>
      </div>
    );
  }
}

const styles = {
  headlineGrid: {
    margin: 10
  },
  headline: {
    height: '100%',
    padding: 10
  },
  inner: {
    height: '100%'
  },
  page: {
    height: '100%',
    marginBottom: 10,
    width: '100%',
    marginRight: 100
  },
  smalltext: {
    color: 'black',
    fontFamily: 'Gill Sans, Gill Sans MT, Calibri, Trebuchet MS, sans-serif',
    fontSize: 18
  },
  titletext: {
    color: 'black',
    fontFamily: 'Gill Sans, Gill Sans MT, Calibri, Trebuchet MS, sans-serif',
    fontSize: 22
  }
};

const mapStateToProps = state => {
  return {
    page: state.ui.page,
    currentDialog: state.ui.currentDialog,
    detailedData: state.ui.detailedData
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeCurrentDialog: type => dispatch(uiChangeCurrentDialog(type)),
    changeNetworkFilesList: list => dispatch(uiSandboxChangeNetworkFilesList(list))
  };
};

const ConnectedSandboxPageContainer = connect(
  mapStateToProps,
  mapDispatchToProps
)(SandboxPageContainer);

export default ConnectedSandboxPageContainer;
