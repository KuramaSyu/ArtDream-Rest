This document describes the Web-Service "ArtDream" which provides a REST API for photography/digital art images and a Website where users can upload their images and rank images. 

# Backend
A Go backend with Gin as REST Framework. it should version its API, e.g. `/api/v1/...`.
- it should provide a POST image endpoint, which accepts image, author and title. Title can be optional
- a get endpoint for all urls of a specific author `/images/author` which then returns a list of URLs + titles. 
- a get endpint for random image: `/images/random` which returns 
  ```[{
  url: string,
  author: string,
  title: string
  }]
  ```
  but it should accept a key, of how many to return. 
- a get endpint for the actual image by the given url -> `/image/id`
  - this should accept overrides like format=webp/png/jpg, it should allow w= or h= or both, to rescale. By default return a webp of in original quality
- now to the ranking part:
  - requests will be done with GET `images/random` and parameter amount = 2, so nothing new here
  - a POST `/rank/vote` in any reasonable extendable structure. Website will for now say, which image lost and which won. But it will also have a second mode, where Analytic Hierarchy process is used, where user sends 1-5 where 5 means second image is way better and 1 means first image is way better. 3 means both are same off. store it in sqlite. user will also send the 2 image ids with left-img and right-img. it could happen later, that it uses numbers 1-100 instead of 1-5. so you also need a body param for the max and min values, which is then stored as well. for that, a table with the different modes should be created, and a second table with mode + cols for the results (containing user information and metadata)
  - a GET `images/rank?time='day/week/month/year/all/custom` (e.g. timedelta in seconds) and a param for the amount of results returned, which then retuns the according winners
- user endpoints for whatever you need
- ranking endpoints or other endpoints i did not mention, but are needed to satisfy the frontend

# Frontend
Here you can for the general layout use your imagination. It should be way more playful then componay prod like. E.g. animations, microinteractions and so on. For the stack: vite + TS + React + MUI + VibrantJS. For MUI, heavily rely on mui themes. Generate MUI themes out of the displayed images on screen (since there will be multiple, choose random or most present or whatever makes sense). For the generation, look at KuramaSyu/WerSu-Frontend which has a themeGenerator and a CustomThemeImpl. Copy this. you should also find it at ../WerSu-Frontend. This will generate a mui theme with vibrant js image colors.

## launch page
on first boot if no user is created, redirect to the launch page. Otherwise disable it. On the launch page bootstrap the first user account, which will be the first admin account. Only the admin can say which other users get granted admin and which not. Admins can manage ranking and images in any way. E.g. modify rank results, delete whole images. 

## admin page
The first created user will be the admin user. The admin page is to mange rankings and images. E.g. CRUD on them, delete all results of a given time and or user (either user name or image user name) or title regex. 

## ranking page
get 2 images from backend and let user decide which is better. display image 1 at the left and image 2 at the right. It should be possible to just click an image, which would mean 2/5 or 4/5, e.g. not total win or loss. But it should be possible to define the answer more granular at the bottom with a long slider from left image into right image, which would then return 1-100.

## launch page 
write there what ever you want. design it however you want. add a `rank images` button or something like that which redirects to ranking page. in the background of the launchpage display iterate through the todays winners. make it changable to week or month or year if the user want it, but hide it a bit. dont clutter the launch page. keep it simple. as less text as possible

## submit page
users can upload their iamges here. it HAS to be a landscape image for now. The user needs to provide a user name. He does not need to provide a title. The user can be logged in - if so already prefill username but make it changable. If not, store his submits and username in localstorage. If he decides to create an account, then ask, if to connect his submits with his user. also: user submits should contain user id if logged in. Userid != user name. User name can be freely choosen like I said. 

# structure
/frontend and /backend and a docker compose to start it