import * as actions from '../constants/actions';
import assign from 'lodash/assign';

const DATAGRAM_INIT = {
  loading: false,
  list: [],
  col: [],
  message: '',
};

function datagram(state = DATAGRAM_INIT, action) {
  switch (action.type) {
    case actions.RESET_ERROR:
      return assign({}, state, {
        loading: false,
        message: '',
      });
    case actions.DATAGRAM_LIST_REQUEST:
      return assign({}, state, {
        loading: true,
      });
    case actions.DATAGRAM_LIST_SUCCESS:
      console.log('get col->' + action.col);
      return assign({}, state, {
        loading: false,
        list: action.list,
        col: action.col,
      });
    case actions.DATAGRAM_LIST_FAILURE:
      return assign({}, state, {
        loading: false,
        message: action.message,
      });
    default:
      return state;
  }
}

export default datagram;
