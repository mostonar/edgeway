/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

export const pipe = (...fns) => val => fns.reduce((acc, f) => f(acc), val);
export const filter = fn => array => array.filter(fn);

export const idlog = label => val => {

  console.log(`${label}: `, val);

  return val;
};
