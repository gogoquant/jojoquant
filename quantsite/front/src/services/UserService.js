import { get, post } from './instance';
import { UserApi } from './api';

/**
 * get user info
 * get: /user/:name
 *
 * @param {string} name - username
 * @returns {Promise<*>}
 */
export const getUser = name => {
  return get(UserApi.user.replace(/:name/, name), {});
};

/**
 * verify token
 * post: /accesstoken
 *
 * @param {string} token - user token
 * @returns {Promise<*>}
 */
export const verifyAccessToken = token => {
  const params = { accesstoken: token };
  return post(UserApi.accessToken, params);
};

/**
 * create user
 * get: /register
 *
 * @param {string} email - email
 * @param {string} passwd - passwd
 * @param {string} name - name
 * @returns {Promise<*>}
 */
export const createUser = (email, passwd, name) => {
  const params = {
    email: email,
    passwd: passwd,
    name: name,
  };
  return post(UserApi.register, params);
};
