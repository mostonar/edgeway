/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React, { Component } from 'react';
import BasicDialog from './basic-dialog';
import { Grid, GridCell } from '@rmwc/grid';
import { Button } from '@rmwc/button';
import GitHubIcon from '../../../img/logo-github.svg';
import GitLabIcon from '../../../img/logo-gitlab.svg';

import {
  OAUTH_PROVIDER_GITHUB,
  OAUTH_PROVIDER_GITLAB
} from '../../app-constants';

class SignInOAuthDialog extends Component {
  constructor(props) {
    super(props);
    this.state = {
    };
  }

  render() {
    return (
      <BasicDialog
        title={this.props.title}
        open={this.props.open}
        onClose={this.props.onClose}
        onSubmit={this.props.onClose}
        submitLabel = {'Cancel'}
      >
        <Grid style={{ marginBottom: 20 }}>
          <GridCell span={6}>
            <Button style={styles.button} outlined onClick={() => this.props.onSignIn(OAUTH_PROVIDER_GITHUB)}>
              <img style={styles.icon} src={GitHubIcon}/>
              GitHub
            </Button>
          </GridCell>
          <GridCell span={6}>
            <Button style={styles.button} outlined onClick={() => this.props.onSignIn(OAUTH_PROVIDER_GITLAB)}>
              <img style={styles.icon} src={GitLabIcon}/>
              GitLab (EOL Account)
            </Button>
          </GridCell>
        </Grid>
        <span style={styles.text}>
          Authenticating with an external provider will:
          <ul>
            <li>Redirect the browser to the provider login page</li>
            <li>Request authorization to read your public user name</li>
            <li>Create your EdgeNative - Connector on successful login and authorization</li>
          </ul>
          NOTE: Login & authorization may be seamless if already performed.
        </span>
      </BasicDialog>
    );
  }
}

const styles = {
  button: {
    width: '100%',
    height: '48px',
    whiteSpace: 'nowrap'
  },
  icon: {
    height: '75%',
    marginRight: 10
  },
  text: {
    color: 'gray'
  }
};

export default SignInOAuthDialog;
