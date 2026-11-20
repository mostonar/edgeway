/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React from 'react';
import { Typography } from '@rmwc/typography';
import { Grid, GridCell } from '@rmwc/grid';
import { Button } from '@rmwc/button';

import {
  ETSI_PRIVACY_URL,
  ETSI_TERMS_OF_USE_URL,
  ETSI_CONTACT_URL
} from '../app-constants';


const Footer = () => {
  /*eslint-disable */
  const logo = require('../../img/logo-edgenative-connector-white.png');
  /* eslint-enable */

  return (
    <Grid style={{ backgroundColor: '#3e484f' }}>
      <GridCell span={2}/>
      <GridCell span={3} align={'middle'} style={styles.copyright}>
        <Typography use="body1">© Copyright 2021-2026, EdgeNative - Connector</Typography>
      </GridCell>
      <GridCell span={5} align={'middle'}>
        <div align={'right'}>
          <Button
            style={styles.button}
            onClick={() => {
              window.open(ETSI_PRIVACY_URL,'_blank');
            }}
          >
            Privacy
          </Button>
          <span style={styles.separator}>|</span>
          <Button
            style={styles.button}
            onClick={() => {
              window.open(ETSI_TERMS_OF_USE_URL,'_blank');
            }}
          >
            Terms of Use
          </Button>
          <span style={styles.separator}>|</span>
          <Button
            style={styles.button}
            onClick={() => {
              window.open(ETSI_CONTACT_URL,'_blank');
            }}
          >
            Contact
          </Button>
        </div>
      </GridCell>
      <GridCell span={2}/>
    </Grid>
  );
};

const styles = {
  copyright: {
    color: 'white',
    padding: 10
  },
  button: {
    color: 'white',
    fontSize: '0.8rem'
  },
  separator: {
    color: '#586269',
    margin: 3
  }
};

export default Footer;
