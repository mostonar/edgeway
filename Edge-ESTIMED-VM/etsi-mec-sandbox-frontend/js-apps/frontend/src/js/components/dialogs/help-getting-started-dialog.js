/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React, { Component } from 'react';
import { connect } from 'react-redux';
import BasicDialog from './basic-dialog';
import { Typography } from '@rmwc/typography';
import { Checkbox } from '@rmwc/checkbox';

import {
  uiChangeHelpOnSignIn
} from '../../state/ui';

import { withTelemetry } from '../../util/withTelemetry';

class HelpGettingStartedDialog extends Component {
  constructor(props) {
    super(props);
    this.state = {
    };
  }

  changeHelpOnSignInCheckbox() {
    this.props.changeHelpOnSignIn(!this.props.helpOnSignIn);
  }

  render() {
    return (
      <BasicDialog
        title={'Welcome to EdgeNative - Connector portal'}
        open={this.props.open}
        onClose={this.props.onClose}
        onSubmit={this.props.onClose}
        submitLabel = {'Ok'}
        className={'large-dialog'}
      >
        <div>
          <Typography use="body1">
            To help getting started, here's a small guide.
          </Typography>
          <Typography theme="primary" use="body1">
            <p><b>Sign in</b></p>
          </Typography>
          <Typography use="body1">
            <ul>
              <li>To start using EdgeNative - Connector, you first need to sign in</li>
              <li>We offer two sign in options: using your EOL account or a GitHub account</li>
            </ul>
          </Typography>
          <Typography theme="primary" use="body1">
            <p><b>Configure EdgeNative - Connector</b></p>
          </Typography>
          <Typography use="body1">
            <ul>
              <li>Start by selecting a simulated network from the drop down menu</li>
              <li>Terminal devices will appear on the map and start moving</li>
              <li>Terminal devices can be paused, added or removed at anytime</li>
              <li>Map details can be configured</li>
            </ul>
          </Typography>
          <Typography theme="primary" use="body1">
            <p><b>Experiment with MEC APIs</b><br/></p>
          </Typography>
          <Typography use="body1">
            <ul>
              <li>Select a MEC API and click on "Try-it in the browser" link</li>
              <li>The new browser tab contains a MEC API Client</li>
              <li>In the MEC API Client tab, select an endpoint and use the "Try-it now"
                button to send live requests to the MEC APIs in your sandbox</li>
            </ul>
          </Typography>
          <Typography theme="primary" use="body1">
            <p><b>Observe MEC API behavior</b><br/></p>
          </Typography>
          <Typography use="body1">
            <ul>
              <li>The API console shows requests and responses from the MEC APIs</li>
              <li>Click on a request to see details, including parameter values</li>
              <li>Notice that requests and responses reflect the current state of the
                scenario when they were received</li>
            </ul>
          </Typography>
          <Typography theme="primary" use="body1">
            <p><b>If you need more help using the platform</b><br/></p>
          </Typography>
          <Typography use="body1">
            <ul>
              <li>Hover your mouse over Sandbox controls to bring up a tooltip</li>
              <li>Refer to the Wiki page under the HELP menu in the top bar</li>
            </ul>
          </Typography>
        </div>
        <Checkbox
          checked={!this.props.helpOnSignIn}
          onChange={() => this.changeHelpOnSignInCheckbox()}
        >
         Do not show on Login
        </Checkbox>
      </BasicDialog>
    );
  }
}

const mapStateToProps = state => {
  return {
    helpOnSignIn: state.ui.helpOnSignIn
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeHelpOnSignIn: check => dispatch(uiChangeHelpOnSignIn(check))
  };
};

const ConnectedHelpGettingStartedDialog = connect(
  mapStateToProps,
  mapDispatchToProps
)(HelpGettingStartedDialog);

export default withTelemetry(ConnectedHelpGettingStartedDialog, 'HelpGettingStartedDialog');
