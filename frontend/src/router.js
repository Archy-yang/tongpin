import { createRouter, createWebHistory } from 'vue-router'
import ChatView from './views/ChatView.vue'
import StyleView from './views/StyleView.vue'
import MeView from './views/MeView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/chat' },
    { path: '/chat', component: ChatView },
    { path: '/style', component: StyleView },
    { path: '/me', component: MeView },
  ],
})
