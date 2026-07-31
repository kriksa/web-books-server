<template>
    <div class="fit column">
        <div class="col sets-tab-panel">
            <div class="sets-item row">
                <UserHotKeys v-model="form.userHotKeys" />
            </div>
        </div>
    </div>
</template>

<script>
//-----------------------------------------------------------------------------
import vueComponent from '../../../vueComponent.js';

import UserHotKeys from './UserHotKeys/UserHotKeys.vue';

const componentOptions = {
    components: {
        UserHotKeys,
    },
};
class KeysTab {
    _options = componentOptions;
    _props = {
        form: Object,
    };

    created() {
    }

    mounted() {
    }

    get mode() {
        return this.$store.state.config.mode;
    }
}

export default vueComponent(KeysTab);
//-----------------------------------------------------------------------------
</script>

<style scoped>
.label {
    width: 110px;
}

</style>
