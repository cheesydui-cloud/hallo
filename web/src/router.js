import { createRouter, createWebHistory } from 'vue-router'
import Gate from './views/Gate.vue'
import Shell from './views/Shell.vue'
import Users from './views/Users.vue'
import Inbound from './views/Inbound.vue'
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
        { path: '', redirect: '/nodes' },
        { path: 'users', component: Users },
        { path: 'plans', redirect: '/users' },
        { path: 'inbound', redirect: '/inbounds' },
        { path: 'inbounds', component: Inbound },
        { path: 'outbounds', redirect: '/inbounds' },
        { path: 'nodes', component: Nodes },
        { path: 'settings', component: Settings },
      ],
    },
  ],
})

export default router
