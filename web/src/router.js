import { createRouter, createWebHistory } from 'vue-router'
import Gate from './views/Gate.vue'
import Shell from './views/Shell.vue'
import Dashboard from './views/Dashboard.vue'
import Users from './views/Users.vue'
import Plans from './views/Plans.vue'
import Inbound from './views/Inbound.vue'
import Outbound from './views/Outbound.vue'
import Settings from './views/Settings.vue'
import Nodes from './views/Nodes.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Gate },
    {
      path: '/',
      component: Shell,
      children: [
        { path: '', component: Dashboard },
        { path: 'users', component: Users },
        { path: 'plans', component: Plans },
        { path: 'inbound', redirect: '/inbounds' },
        { path: 'inbounds', component: Inbound },
        { path: 'outbounds', component: Outbound },
        { path: 'nodes', component: Nodes },
        { path: 'settings', component: Settings },
      ],
    },
  ],
})

export default router
