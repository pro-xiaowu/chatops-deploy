import '@testing-library/jest-dom/vitest'

const getComputedStyle=window.getComputedStyle
Object.defineProperty(window,'getComputedStyle',{
  configurable:true,
  value:(element:Element)=>getComputedStyle(element),
})

Object.defineProperty(window,'matchMedia',{
  writable:true,
  value:(query:string)=>({
    matches:false,
    media:query,
    onchange:null,
    addListener:()=>{},
    removeListener:()=>{},
    addEventListener:()=>{},
    removeEventListener:()=>{},
    dispatchEvent:()=>false,
  }),
})

class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

window.ResizeObserver=ResizeObserver
