/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import _ from 'lodash';
import { connect } from 'react-redux';
import React, { Component } from 'react';
import { Select } from '@rmwc/select';
import TryIt from './try-it';
import AppInstanceInfo from './app-instance-info';
import ApiTable from './api-table';
import { Grid, GridCell } from '@rmwc/grid';
import { Typography } from '@rmwc/typography';

import {
  uiSandboxChangeMecApiSelected
} from '../../state/ui';

import {
  DEFAULT_NO_NETWORK_FILE_SELECTED
} from '../../app-constants';

class ApiPane extends Component {
  constructor(props) {
    super(props);
  }

  getSelectedEdgeApp() {
    if (this.props.mecApiSelected && this.props.edgeApps) {
      for (var i = 0; i < this.props.edgeApps.length; i++) {
        let edgeApp = this.props.edgeApps[i];
        if (edgeApp.pseudoName === this.props.mecApiSelected) {
          return edgeApp;
        }
      }
    }
    return null;
  }

  getMecApiOptions(edgeApps) {
    var mepOptions = {};
    if (edgeApps) {
      for (var i = 0; i < edgeApps.length; i++) {
        let edgeApp = edgeApps[i];
        if (!mepOptions[edgeApp.mepName]) {
          mepOptions[edgeApp.mepName] = [];
        }
        mepOptions[edgeApp.mepName].push({label: edgeApp.dropDownName, value: edgeApp.pseudoName});
      }
    }
    var options = [];
    _.forOwn(mepOptions, function(value, key) {
      var orderedValues = _.orderBy(value, 'label', 'asc');
      var entry = {
        label: key,
        options: orderedValues
      };
      options.push(entry);
    });
    return _.orderBy(options, 'label', 'asc');
  }

  render() {

    // Get selected Edge App
    let selectedEdgeApp = this.getSelectedEdgeApp();

    const configEnabled = (this.props.networkFileSelected !== DEFAULT_NO_NETWORK_FILE_SELECTED) ? true : false;

    // Refresh MEC API select options
    var options = this.getMecApiOptions(this.props.edgeApps);
    let selectedOption = selectedEdgeApp ? selectedEdgeApp.pseudoName : '';

    return (
      <div>
        <div style={styles.title} title="Observe and interact with MEC APIs">
          <Typography theme="primary" use="headline6">API Console</Typography>
        </div>
        <div style={{ display: 'flex', height: '100%', marginBottom: 5 }}>
          <div style={{ flex: '1', marginRight: 5, padding: 10 }}>
            <ApiTable/>
          </div>
          <div
            className="component-style"
            style={{ flex: '0 0 450px' }}
          >
            <div style={styles.section}>
              <Grid>
                <GridCell span={12}>
                  <Select
                    title="Select the MEC API"
                    style={styles.select}
                    fullwidth="true"
                    outlined
                    label="MEC Service APIs"
                    value={selectedOption}
                    options={options}
                    disabled={!configEnabled}
                    onChange={event => {
                      this.props.changeMecApiSelected(event.target.value);
                    }}
                  />
                </GridCell>
              </Grid>
            </div>

            <div style={styles.section}>
              <AppInstanceInfo
                edgeApp={selectedEdgeApp}
                applicationsApi={this.props.applicationsApi}
                eventsApi={this.props.eventsApi}
                disabled={!configEnabled}
              />
            </div>

            <div style={styles.section}>
              <TryIt
                edgeApp={selectedEdgeApp}
                disabled={!configEnabled}
              />
            </div>
          </div>
        </div>
      </div>
    );
  }
}

const styles = {
  title: {
    paddingLeft: 15,
    paddingTop: 15
  },
  section: {
    padding: 5,
    paddingRight: 15,
    marginLeft: 10
  },
  select: {
    width: '100%',
    color: '#2f00ff'
  }
};

const mapStateToProps = state => {
  return {
    sandboxName: state.ui.sandboxName,
    mecApiSelected: state.ui.mecApiSelected,
    networkFileSelected: state.ui.networkFileSelected,
    activationInProgressCount: state.ui.activationInProgressCount,
    scenario: state.sbox.scenario,
    appInstanceTable: state.sbox.appInstanceTable.data,
    edgeApps: state.ui.edgeApps
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeMecApiSelected: name => dispatch(uiSandboxChangeMecApiSelected(name))
  };
};

const ConnectedApiPane = connect(
  mapStateToProps,
  mapDispatchToProps
)(ApiPane);

export default ConnectedApiPane;
