/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React, { Component } from 'react';
import BasicDialog from './basic-dialog';
import { DLG_CONFIRM_DELETE_APP } from '../../app-constants';

class ConfirmDeleteAppDialog extends Component {
  constructor(props) {
    super(props);
  }

  deleteConfirmed() {
    this.props.onSubmit();
  }

  onClose(/*closeFromSubmit*/) {
    this.props.onClose();
  }

  render() {
    return (
      <BasicDialog
        title={this.props.title}
        open={this.props.open}
        onSubmit={() => this.deleteConfirmed()}
        onClose={(closeFromSubmit) => this.onClose(closeFromSubmit)}
        closeLabel = {'CANCEL'}
        submitLabel = {'DELETE'}
        cydata={DLG_CONFIRM_DELETE_APP}
      >

        <p>Are you sure you want to delete the selected MEC Application instance?</p>

      </BasicDialog>
    );
  }
}

export default ConfirmDeleteAppDialog;
