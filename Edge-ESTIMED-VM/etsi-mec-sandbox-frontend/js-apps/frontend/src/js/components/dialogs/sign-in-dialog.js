/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React, { Component } from 'react';
import { TextField, TextFieldHelperText } from '@rmwc/textfield';
import { Typography } from '@rmwc/typography';
import BasicDialog from './basic-dialog';
import { DLG_SIGN_IN } from '../../app-constants';

class SignInDialog extends Component {
  constructor(props) {
    super(props);
    this.state = {
      userName: null,
      userErr: null,
      password: null,
      passwordErr: null
    };
  }

  changeUserName(name) {
    var err = null;

    if (name) {
      if (name.length > 30) {
        err = 'Maximum 30 characters';
      } else if (!name.match(/^(([A-Za-z0-9][-A-Za-z0-9.]*)?[@.-A-Za-z0-9])+$/)) {
        err = 'Alphanumeric or \'@.-\'';
      }
    } else {
      err = 'Please enter a user name';
    }
    this.setState({
      userName: name,
      userErr: err
    });
  }

  changePassword(psw) {
    var err = null;

    if (psw) {
      if (psw.length > 30) {
        err = 'Maximum 30 characters';
      }
    }
    this.setState({
      password: psw,
      passwordErr: err
    });
  }

  submitUser() {
    this.props.onSubmit(this.state);
    this.state.password = '';
  }

  onClose(closeFromSubmit) {
    this.props.onClose();
    if (!closeFromSubmit) {
      this.state.password = '';
    }
  }

  render() {
    return (
      <BasicDialog
        title={this.props.title}
        open={this.props.open}
        onSubmit={() => this.submitUser()}
        onClose={(closeFromSubmit) => this.onClose(closeFromSubmit)}
        closeLabel = {'CANCEL'}
        submitLabel = {'SIGN IN'}
        okDisabled={
          (!this.state.userName) || this.state.userErr ||
          (!this.state.password) || this.state.passowrdErr
        }
        cydata={DLG_SIGN_IN}
      >

        { this.props.errorMsg &&
          <Typography use="body1" style={styles.errorText}>{this.props.errorMsg}</Typography>
        }

        <TextField
          outlined
          style={{ width: '100%' }}
          label={'User Name'}
          invalid={
            this.state.userErr
          }
          value={this.state.userName}
          onChange={e => this.changeUserName(e.target.value)}
        />
        <TextFieldHelperText validationMsg={true}>
          <span>{this.state.userErr}</span>
        </TextFieldHelperText>

        <TextField
          type='password'
          outlined
          style={{ width: '100%' }}
          label={'Password'}
          value={this.state.password}
          invalid={
            this.state.passwordErr
          }
          onChange={e => this.changePassword(e.target.value)}
        />
        <TextFieldHelperText validationMsg={true}>
          <span>{this.state.passwordErr}</span>
        </TextFieldHelperText>

      </BasicDialog>
    );
  }
}

const styles = {
  errorText: {
    color: 'red'
  }
};

export default SignInDialog;
