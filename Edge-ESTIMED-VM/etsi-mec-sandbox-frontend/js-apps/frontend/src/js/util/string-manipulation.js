/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import { MEC_SERVICES } from '../app-constants';

export const firstLetterUpper = str => {
  if (!str) {
    return '';
  }
  return str.charAt(0).toUpperCase() + str.slice(1);
};

export const camelCasePrefix = prefix => {
  if (!prefix) {
    return '';
  }
  const array = prefix.split('-');
  var f = array[0];
  array[0] = f.charAt(0).toLowerCase() + f.slice(1);
  return array.join('');
};

export const mapService = loggerName => {
  if (!loggerName) {
    return '';
  }
  const svc = MEC_SERVICES.find(s => loggerName.startsWith(s.prefix));
  if (svc) {
    return loggerName.replace(svc.prefix, svc.code);
  }
  return 'N/A' + loggerName;
};
