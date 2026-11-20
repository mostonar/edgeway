/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React from 'react';
import { Typography } from '@rmwc/typography';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  DialogButton
} from '@rmwc/dialog';

const BasicDialog = props => {
  return (
    <Dialog
      style={{ zIndex: 10000 }}
      open={props.open}
      onClose={(evt) => {
        var closeOnSubmit = (evt.detail.action === 'accept' ? true : false);
        props.onClose(closeOnSubmit);
      }}
      className={props.className ? props.className : ''}
    >
      <DialogTitle theme="primary" style={styles.title}>
        <Typography use="headline5">{props.title}</Typography>
      </DialogTitle>

      <DialogContent style={styles.content}>
        {props.children}
      </DialogContent>

      <DialogActions style={styles.actions}>
        {props.closeLabel && (
          <DialogButton
            style={styles.button}
            action="close"
            onClick={props.onClose}
          >
            {props.closeLabel}
          </DialogButton>
        )}
        {props.submitLabel && (
          <DialogButton
            style={styles.button}
            action="accept"
            isDefaultAction
            onClick={() => props.onSubmit()}
            disabled={props.okDisabled}
          >
            {props.submitLabel}
          </DialogButton>
        )}
      </DialogActions>
    </Dialog>
  );
};

const styles = {
  title: {
    paddingTop: 10,
    paddingBottom: 15
  },
  actions: {
    paddingBottom: 10,
    paddingRight: 24
  },
  button: {
    marginBottom: 5,
    marginLeft: 10
  }
};

export default BasicDialog;
