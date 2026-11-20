/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React, { Component } from 'react';
import { TextField, TextFieldHelperText } from '@rmwc/textfield';
import { Select } from '@rmwc/select';
import BasicDialog from './basic-dialog';
import { Grid, GridCell } from '@rmwc/grid';
import { DLG_CREATE_NEW_APP } from '../../app-constants';

class CreateNewAppDialog extends Component {
  constructor(props) {
    super(props);
    this.state = {
      appName: '',
      appErr: null,
      mepName: ''
    };
  }

  changeAppName(name) {
    var err = null;

    if (name) {
      if (name.length > 20) {
        err = 'Maximum 20 characters';
      } else if (!name.match(/^(([A-Za-z0-9][-A-Za-z0-9.]*)?[@.-A-Za-z0-9])+$/)) {
        err = 'Alphanumeric or \'@.-\'';
      }
    } else {
      err = 'Please enter an application name';
    }
    this.setState({
      appName: name,
      appErr: err
    });
  }

  changeMepName(name) {
    this.setState({
      mepName: name
    });
  }

  handleKeyPress(e) {
    if (e.key === 'Enter') {
      if (!(this.state.appName === '' || this.state.appErr || this.state.mepName === '')) {
        this.submitApp();
      }
    }
  }

  submitApp() {
    this.props.onSubmit(this.state);
  }

  onClose(closeFromSubmit) {
    if (closeFromSubmit === true) {
      this.submitApp();
    }
    this.props.onClose();
    this.setState({
      appName: '',
      appErr: null,
      mepName: ''
    });
  }

  render() {
    return (
      <BasicDialog
        title={this.props.title}
        open={this.props.open}
        onSubmit={() => this.submitApp()}
        onClose={(closeFromSubmit) => this.onClose(closeFromSubmit)}
        closeLabel = {'CANCEL'}
        submitLabel = {'CREATE'}
        okDisabled={
          (this.state.appName === '' || this.state.appErr || this.state.mepName === '')
        }
        cydata={DLG_CREATE_NEW_APP}
      >
        <Grid style={{ marginBottom: 5 }}>
          <GridCell span={12}>
            <span style={styles.text}>
              <p style={{ marginBottom: 20 }}>
                Create a unique Application Instance Id in EdgeNative - Connector.<br/>
                Every instance of the same MEC Application should consistently have<br/>
                the same MEC Application Name across MEC platforms.
              </p>
              MEC application:
              {/* <i><b>Note:</b> use a consistent application name for all instances of a<br/>MEC application on each platform</i> */}
            </span>
            <TextField
              style={{ width: '100%', marginTop: 5 }}
              label={'MEC Application Name'}
              invalid={this.state.appErr ? true : false}
              value={this.state.appName}
              onChange={e => this.changeAppName(e.target.value)}
              onKeyPress={e => this.handleKeyPress(e)}
            />
            <TextFieldHelperText validationMsg={true}>
              <span>{this.state.appErr}</span>
            </TextFieldHelperText>
          </GridCell>
          <GridCell span={12}>
            <span style={styles.text}>
              MEC platform where the MEC application instance is running:
            </span>
            <Select
              title="Select a MEC platform"
              style={{ width: '100%', marginTop: 10 }}
              label="MEC platform"
              fullwidth="true"
              disabled={false}
              value={this.state.mepName}
              options={this.props.mepNames}
              onChange={mepName => {
                this.changeMepName(mepName.target.value);
              }}
            />
          </GridCell>
        </Grid>
      </BasicDialog>
    );
  }
}

const styles = {
  text: {
    color: 'gray'
  }
};

export default CreateNewAppDialog;
