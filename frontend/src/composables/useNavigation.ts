import { computed, onUnmounted, ref } from 'vue'
import { aboutPages, type AboutPage } from '../config/about'

type Route = 'home' | 'my' | AboutPage
function isAbout(route: string): route is AboutPage {
  return aboutPages.some((page) => page.id === route)
}
function readRoute(): Route {
  const path = location.hash.slice(1)
  if (path === '/my') return 'my'
  const detail = path.startsWith('/my/') ? path.slice(4) : ''
  return isAbout(detail) ? detail : 'home'
}

export function useNavigation() {
  const route = ref<Route>(readRoute())
  function sync() {
    route.value = readRoute()
    window.scrollTo(0, 0)
  }
  window.addEventListener('popstate', sync)
  window.addEventListener('hashchange', sync)
  onUnmounted(() => {
    window.removeEventListener('popstate', sync)
    window.removeEventListener('hashchange', sync)
  })
  function navigate(next: Route, replace = false) {
    if (next === route.value) return
    const detail = isAbout(next)
    const replacingDetail = detail && isAbout(route.value)
    const state = {
      panguaiBackToMy:
        detail &&
        (route.value === 'my' || (replacingDetail && history.state?.panguaiBackToMy === true)),
    }
    const hash = detail ? `#/my/${next}` : `#/${next}`
    if (replace || replacingDetail) history.replaceState(state, '', hash)
    else history.pushState(state, '', hash)
    sync()
  }
  function backToMy() {
    if (history.state?.panguaiBackToMy === true) history.back()
    else navigate('my', true)
  }
  return {
    page: computed(() => (route.value === 'home' ? 'home' : 'my')),
    aboutPage: computed(() => (isAbout(route.value) ? route.value : null)),
    navigate,
    backToMy,
  }
}
