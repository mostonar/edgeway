/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { connect } from 'react-redux';
import React, { Component } from 'react';
import { Typography } from '@rmwc/typography';
import { TextField, TextFieldIcon } from '@rmwc/textfield';
import { Grid, GridCell } from '@rmwc/grid';

import {
  HOST_PATH
} from '../../app-constants';

import {
  uiSandboxChangeMecApiCopiedToClipboard
} from '../../state/ui';

class TryIt extends Component {
  constructor(props) {
    super(props);
    this.baseTryIt = HOST_PATH + '/' + this.props.sandboxName + '/';

    // Initialize undefined states
    if (this.props.mecApiCopiedToClipboard) {
      this.props.changeMecApiCopiedToClipboard(false);
    }
  }

  componentDidUpdate(prevProps) {
    if (this.props.mecApiSelected !== prevProps.mecApiSelected) {
      this.props.changeMecApiCopiedToClipboard(false);
    }
  }

  copyToClipboard(copyText) {
    // Copy the text inside the text field
    navigator.clipboard.writeText(copyText);
    // Update icon state
    this.props.changeMecApiCopiedToClipboard(true);
  }

  render() {
    if (this.props.disabled || !this.props.edgeApp) {
      return null;
    }

    // Make sure Service is enabled
    let edgeApp = this.props.edgeApp;
    if (!edgeApp.enabled || edgeApp.enableInProgressCount !== -1 || edgeApp.disableInProgressCount !== -1) {
      return null;
    }

    let tryItMecApp = this.baseTryIt + edgeApp.path;
    let tryItBrowser = this.baseTryIt + edgeApp.url;
    let clipboardIcon = this.props.mecApiCopiedToClipboard ? 'assignment_turned_in' : 'assignment';

    return (
      <>
        <div style={{ width: '100%', borderTop: '2px solid #e4e4e4', marginTop: 15, marginBottom: 15 }}/>

        <div title="Open a browser client to exercise selected MEC API">
          <Typography theme="primary" use="headline6">Try-it in the browser</Typography>
          <Typography className="text-color-main" use="body1">
            <p>
              Click <b><a href={tryItBrowser} target="_blank">here</a></b> to start a MEC client in the browser.
            </p>
            <p>
              The client allows to send HTTP requests to MEC Service APIs in your EdgeNative - Connector sandbox.
            </p>
          </Typography>
        </div>

        <div style={{ width: '100%', borderTop: '2px solid #e4e4e4', marginTop: 15, marginBottom: 15 }}/>

        <div title="Use URL in your client to exercise selected MEC API">
          <Typography theme="primary" use="headline6">Try-it from your User application</Typography>
          <Typography className="text-color-main" use="body1">
            <p>
              Use the following base path in your MEC application to interact directly with the MEC Service API in your EdgeNative - Connector sandbox.
            </p>
          </Typography>
        </div>
        <div>
          <Grid style={{ marginBottom: 30 }}>
            <GridCell span={12}>
              <TextField
                label="MEC Service base path"
                style={{ width: '100%', color: 'gray' }}
                value={tryItMecApp}
                withTrailingIcon={
                  <TextFieldIcon
                    tabIndex="0"
                    icon={clipboardIcon}
                    onClick={() => this.copyToClipboard(tryItMecApp)}
                  />
                }
                onChange={e => this.changeAppName(e.target.value)}
              />
            </GridCell>
          </Grid>
        </div>
      </>
    );
  }
}

const mapStateToProps = state => {
  return {
    sandboxName: state.ui.sandboxName,
    mecApiSelected: state.ui.mecApiSelected,
    mecApiCopiedToClipboard: state.ui.mecApiCopiedToClipboard
  };
};

const mapDispatchToProps = dispatch => {
  return {
    changeMecApiCopiedToClipboard: name => dispatch(uiSandboxChangeMecApiCopiedToClipboard(name))
  };
};

const ConnectedTryIt = connect(
  mapStateToProps,
  mapDispatchToProps
)(TryIt);

export default ConnectedTryIt;
