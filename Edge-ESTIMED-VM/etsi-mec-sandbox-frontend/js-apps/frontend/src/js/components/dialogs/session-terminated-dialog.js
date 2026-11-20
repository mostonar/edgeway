/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React, { Component } from 'react';
import BasicDialog from './basic-dialog';

class SessionTerminatedDialog extends Component {
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
        submitLabel = {'Ok'}
      >
        <div style={styles.text}>
          Your session has terminated. Please sign in again to access your Sandbox.
        </div>
      </BasicDialog>
    );
  }
}

const styles = {
  text: {
    color: 'gray'
  }
};

export default SessionTerminatedDialog;
