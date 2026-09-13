import React from 'react';

export interface IconProps extends React.SVGProps<SVGSVGElement> {
  size?: number;
}

export function InboundsIcon({ size = 20, ...props }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      {...props}
    >
      <path
        d="M22 6H10C7.8 6 6 7.8 6 10V22C6 24.2 7.8 26 10 26H22C24.2 26 26 24.2 26 22V10C26 7.8 24.2 6 22 6Z"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeMiterlimit="10"
      />
      <path
        d="M16 11V21M16 21L12 17M16 21L20 17"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function ClientsIcon({ size = 20, ...props }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 33"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      {...props}
    >
      <g clipPath="url(#clip_clients)">
        <path
          d="M15.8531 15.1213C17.9223 15.1213 19.5998 13.4438 19.5998 11.3746C19.5998 9.30537 17.9223 7.62793 15.8531 7.62793C13.7839 7.62793 12.1064 9.30537 12.1064 11.3746C12.1064 13.4438 13.7839 15.1213 15.8531 15.1213Z"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeMiterlimit="10"
        />
        <path
          d="M22.1461 25.6813C24.2153 25.6813 25.8927 24.0039 25.8927 21.9347C25.8927 19.8654 24.2153 18.188 22.1461 18.188C20.0769 18.188 18.3994 19.8654 18.3994 21.9347C18.3994 24.0039 20.0769 25.6813 22.1461 25.6813Z"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeMiterlimit="10"
        />
        <path
          d="M9.85311 25.841C11.9223 25.841 13.5998 24.1636 13.5998 22.0943C13.5998 20.0251 11.9223 18.3477 9.85311 18.3477C7.78389 18.3477 6.10645 20.0251 6.10645 22.0943C6.10645 24.1636 7.78389 25.841 9.85311 25.841Z"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeMiterlimit="10"
        />
      </g>
      <defs>
        <clipPath id="clip_clients">
          <rect width="32" height="32" fill="white" transform="translate(0 0.734863)" />
        </clipPath>
      </defs>
    </svg>
  );
}

export function GroupsIcon({ size = 20, ...props }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      {...props}
    >
      <rect x="6" y="9" width="14" height="15" rx="3" stroke="currentColor" strokeWidth="1.6" />
      <path d="M12 6h11a3 3 0 0 1 3 3v12" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
      <circle cx="11" cy="14" r="1.5" fill="currentColor" />
    </svg>
  );
}

export function NodesIcon({ size = 20, ...props }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      {...props}
    >
      <g clipPath="url(#clip_nodes)">
        <path
          d="M13.5332 15.1468L5.34656 10.7201C4.18656 10.0934 4.18656 8.42675 5.34656 7.80008L13.5332 3.37342C15.0799 2.54675 16.9332 2.54675 18.4666 3.37342L26.6532 7.80008C27.8132 8.42675 27.8132 10.0934 26.6532 10.7201L18.4666 15.1468C16.9199 15.9734 15.0666 15.9734 13.5332 15.1468Z"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeMiterlimit="10"
        />
        <path
          d="M23.1326 12.6406L26.6659 14.5473C27.8259 15.174 27.8259 16.8406 26.6659 17.4673L18.4793 21.894C16.9332 22.7206 15.0793 22.7206 13.5459 21.894L5.35926 17.4673C4.19926 16.8406 4.19926 15.174 5.35926 14.5473L8.89259 12.6406"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeMiterlimit="10"
        />
        <path
          d="M23.1326 19.3594L26.6659 21.266C27.8259 21.8927 27.8259 23.5594 26.6659 24.186L18.4793 28.6127C16.9332 29.4394 15.0793 29.4394 13.5459 28.6127L5.35926 24.186C4.19926 23.5594 4.19926 21.8927 5.35926 21.266L8.89259 19.3594"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeMiterlimit="10"
        />
      </g>
      <defs>
        <clipPath id="clip_nodes">
          <rect width="32" height="32" fill="white" />
        </clipPath>
      </defs>
    </svg>
  );
}

export function SettingsIcon({ size = 20, ...props }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      {...props}
    >
      <path d="M6 10h20M6 22h20" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
      <circle cx="12" cy="10" r="3.5" fill="currentColor" stroke="currentColor" strokeWidth="1" />
      <circle cx="20" cy="22" r="3.5" fill="currentColor" stroke="currentColor" strokeWidth="1" />
    </svg>
  );
}

export function ApiDocsIcon({ size = 20, ...props }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      {...props}
    >
      <g clipPath="url(#clip_api)">
        <path
          d="M22.2667 4H9.73333C6.5669 4 4 6.5669 4 9.73333V22.2667C4 25.4331 6.5669 28 9.73333 28H22.2667C25.4331 28 28 25.4331 28 22.2667V9.73333C28 6.5669 25.4331 4 22.2667 4Z"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeMiterlimit="10"
        />
        <path
          d="M13.6261 11.7734L9.39941 16.0001L13.6261 20.2268"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeMiterlimit="10"
        />
        <path
          d="M18.373 11.7734L22.5997 16.0001L18.373 20.2268"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeMiterlimit="10"
        />
      </g>
      <defs>
        <clipPath id="clip_api">
          <rect width="32" height="32" fill="white" />
        </clipPath>
      </defs>
    </svg>
  );
}
