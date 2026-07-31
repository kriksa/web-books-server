import { createStore } from 'vuex';
//import createPersistedState from 'vuex-persistedstate';
import VuexPersistence from 'vuex-persist';

import root from './root.js';
import config from './modules/config';
import reader from './modules/reader';

const debug = process.env.NODE_ENV !== 'production';

/** Явно localStorage — настройки и состояние reader/config не должны теряться между сеансами. */
const vuexLocal = new VuexPersistence({
    key: 'vuex',
    storage: typeof window !== 'undefined' ? window.localStorage : undefined,
});

export default createStore(Object.assign({}, root, {
    modules: {
        config,
        reader,
    },
    strict: debug,
    plugins: [vuexLocal.plugin]
}));
