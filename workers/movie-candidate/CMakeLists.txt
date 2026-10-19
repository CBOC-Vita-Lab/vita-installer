cmake_minimum_required(VERSION 3.20)
project(vita_movie_candidate LANGUAGES CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_MSVC_RUNTIME_LIBRARY MultiThreaded)

if(NOT CMAKE_CXX_COMPILER_ID MATCHES "Clang")
  message(FATAL_ERROR "Use Clang with the reviewed scalar floating-point recipe.")
endif()
add_executable(vita-movies
  ../vendor/galaxy/tools/thp_transcode.cpp
  ../galaxy-host-log.cpp)
target_include_directories(vita-movies PRIVATE "${JPEG_SOURCE_DIR}/src" "${JPEG_BUILD_DIR}")
target_link_libraries(vita-movies PRIVATE "${JPEG_STATIC_LIBRARY}")
if(MSVC)
  target_compile_options(vita-movies PRIVATE /O2 /U__ARM_NEON /U__ARM_NEON__ /clang:-ffp-contract=off)
else()
  target_compile_options(vita-movies PRIVATE -O2 -g0 -U__ARM_NEON -U__ARM_NEON__ -ffp-contract=off)
endif()
set_target_properties(vita-movies PROPERTIES RUNTIME_OUTPUT_DIRECTORY "${CANDIDATE_OUTPUT_DIR}"
  RUNTIME_OUTPUT_DIRECTORY_RELEASE "${CANDIDATE_OUTPUT_DIR}")
